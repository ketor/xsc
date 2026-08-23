package session

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionAmbiguous = errors.New("session path is ambiguous")
)

// PasswordDecrypter 密码解密接口
type PasswordDecrypter interface {
	Decrypt(encrypted, master string) (string, error)
}

// DecrypterFunc 函数适配器，将普通函数转为 PasswordDecrypter 接口
type DecrypterFunc func(encrypted, master string) (string, error)

// Decrypt 实现 PasswordDecrypter 接口
func (f DecrypterFunc) Decrypt(encrypted, master string) (string, error) {
	return f(encrypted, master)
}

var (
	decryptersMu sync.RWMutex
	decrypters   = make(map[string]PasswordDecrypter)
)

// RegisterDecrypter 注册密码解密器
func RegisterDecrypter(source string, d PasswordDecrypter) {
	decryptersMu.Lock()
	defer decryptersMu.Unlock()
	decrypters[source] = d
}

// GetDecrypter 获取已注册的密码解密器
func GetDecrypter(source string) (PasswordDecrypter, bool) {
	decryptersMu.RLock()
	defer decryptersMu.RUnlock()
	d, ok := decrypters[source]
	return d, ok
}

// AuthType 定义认证类型
type AuthType string

const (
	AuthTypePassword AuthType = "password"
	AuthTypeKey      AuthType = "key"
	AuthTypeAgent    AuthType = "agent"
)

// keyAuthAliases 列出 auth_type 中所有应被规范化为 AuthTypeKey 的标准/常见别名。
//
// 来源：
//   - "publickey" 是 SSH 协议中 USERAUTH_REQUEST 使用的标准方法名，OpenSSH 配置文件
//     PreferredAuthentications 也用这个值，SecureCRT/Xshell 等客户端导出格式同样使用。
//   - "rsa"/"dsa"/"ecdsa"/"ed25519" 是具体密钥算法名。SecureCRT 的 Authentication
//     字段把它们当作 publickey 的同义词（见 internal/securecrt/parser.go
//     parseAuthMethods 的实际映射），用户手写 YAML 时也常用这些标识。
//   - 全部规范化到内部规范的 AuthTypeKey，让 Validate / 上层渲染路径不必各自重复识别。
var keyAuthAliases = map[AuthType]bool{
	"publickey": true,
	"rsa":       true,
	"dsa":       true,
	"ecdsa":     true,
	"ed25519":   true,
}

// AuthMethod 定义认证方法配置
type AuthMethod struct {
	Type              string `yaml:"type"`                         // 认证类型: password, key, agent, keyboard-interactive
	Priority          int    `yaml:"priority,omitempty"`           // 优先级顺序
	Password          string `yaml:"password,omitempty"`           // 密码（用于 password 类型）
	EncryptedPassword string `yaml:"encrypted_password,omitempty"` // 加密密码（SecureCRT 延迟解密）
	KeyPath           string `yaml:"key_path,omitempty"`           // 密钥路径（用于 key 类型）
}

// Session 定义 SSH 会话配置
type Session struct {
	Host        string       `yaml:"host"`
	Port        int          `yaml:"port"`
	User        string       `yaml:"user"`
	AuthType    AuthType     `yaml:"auth_type"`
	Password    string       `yaml:"password,omitempty"`
	KeyPath     string       `yaml:"key_path,omitempty"`
	Description string       `yaml:"description,omitempty"`
	ProxyJump   string       `yaml:"proxy_jump,omitempty"`   // 跳板机会话路径（如 "极光云/10.220.75.62 (zetyun)"）
	AuthMethods []AuthMethod `yaml:"auth_methods,omitempty"` // 认证方法列表（按优先级）

	// 内部字段
	FilePath          string `yaml:"-"`
	Name              string `yaml:"-"`
	Valid             bool   `yaml:"-"`
	Error             error  `yaml:"-"`
	EncryptedPassword string `yaml:"-"` // 加密密码（延迟解密）
	MasterPassword    string `yaml:"-"` // 主密码（用于解密）
	PasswordSource    string `yaml:"-"` // 密码来源："securecrt"、"xshell" 或 "mobaxterm"
}

// Validate 验证会话配置是否有效
func (s *Session) Validate() error {
	if s.Host == "" {
		return fmt.Errorf("host is required")
	}
	if s.Port == 0 {
		s.Port = 22
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if s.User == "" {
		s.User = os.Getenv("USER")
		if s.User == "" {
			s.User = "root"
		}
	}
	if s.AuthType == "" {
		s.AuthType = AuthTypeAgent
	}
	// 规范化 SSH 标准术语和常见密钥算法名为内部 AuthTypeKey
	// （详见 keyAuthAliases 上的注释）。
	if keyAuthAliases[s.AuthType] {
		s.AuthType = AuthTypeKey
	}

	switch s.AuthType {
	case AuthTypePassword:
		// 密码可以为空，连接时交互式输入
	case AuthTypeKey:
		// key_path 为空时连接层会自动回退到 ~/.ssh/ 下的默认密钥（id_ed25519/id_rsa 等），
		// 见 ssh/client.go findDefaultSSHKeys。所以这里只在显式给了 key_path 但路径不存在
		// 时才报错。
		if s.KeyPath != "" {
			if s.KeyPath[0] == '~' {
				home, err := os.UserHomeDir()
				if err == nil {
					s.KeyPath = filepath.Join(home, s.KeyPath[1:])
				}
			}
			if _, err := os.Stat(s.KeyPath); os.IsNotExist(err) {
				return fmt.Errorf("key file not found: %s", s.KeyPath)
			}
		}
	case AuthTypeAgent:
		// Agent 认证不需要额外配置
	default:
		return fmt.Errorf("invalid auth_type: %s", s.AuthType)
	}

	return nil
}

// ResolvePassword 延迟解密密码（用于 SecureCRT / XShell 会话）
// 根据 PasswordSource 选择对应的解密器
// 环境变量 XSC_MASTER_PASSWORD 优先于配置文件中的主密码
func (s *Session) ResolvePassword() error {
	if s.Password != "" || s.EncryptedPassword == "" {
		return nil
	}
	// XSC_MASTER_PASSWORD 环境变量覆盖配置文件主密码
	if envMaster := os.Getenv("XSC_MASTER_PASSWORD"); envMaster != "" {
		s.MasterPassword = envMaster
	}
	if s.MasterPassword == "" {
		return fmt.Errorf("master password not set for decryption")
	}

	var decrypted string
	var err error

	d, ok := GetDecrypter(s.PasswordSource)
	if !ok {
		return fmt.Errorf("unknown password source: %q (no decrypter registered)", s.PasswordSource)
	}
	decrypted, err = d.Decrypt(s.EncryptedPassword, s.MasterPassword)

	if err != nil {
		return fmt.Errorf("failed to decrypt password: %w", err)
	}
	s.Password = decrypted
	return nil
}

// DisplayName 返回会话的显示名称
func (s *Session) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Host
}

// LoadSession 从 YAML 文件加载会话配置
func LoadSession(filePath string) (*Session, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var session Session
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&session); err != nil {
		session.FilePath = filePath
		session.Name = filepath.Base(filePath)
		session.Name = session.Name[:len(session.Name)-len(filepath.Ext(session.Name))]
		session.Valid = false
		session.Error = fmt.Errorf("invalid YAML format: %w", err)
		return &session, nil
	}

	session.FilePath = filePath
	session.Name = filepath.Base(filePath)
	session.Name = session.Name[:len(session.Name)-len(filepath.Ext(session.Name))]

	if err := session.Validate(); err != nil {
		session.Valid = false
		session.Error = err
	} else {
		session.Valid = true
	}

	return &session, nil
}

// SaveSession 保存会话配置到 YAML 文件
func SaveSession(session *Session, filePath string) error {
	// 确保目录存在
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ResolveSessionFile 将用户提供的会话路径解析为 sessionsDir 内的文件。
// 它拒绝绝对路径、路径穿越和逃逸根目录的符号链接。
func ResolveSessionFile(sessionsDir, input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("session path is required")
	}
	if strings.ContainsRune(input, '\x00') {
		return "", fmt.Errorf("session path contains NUL")
	}

	normalized := filepath.FromSlash(input)
	if filepath.IsAbs(normalized) || filepath.VolumeName(normalized) != "" {
		return "", fmt.Errorf("absolute session path is not allowed: %s", input)
	}
	clean := filepath.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("session path escapes sessions directory: %s", input)
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if ext != ".yaml" && ext != ".yml" {
		clean += ".yaml"
	}

	rootAbs, err := filepath.Abs(sessionsDir)
	if err != nil {
		return "", fmt.Errorf("resolve sessions directory: %w", err)
	}
	target := filepath.Join(rootAbs, clean)
	if err := ensurePathWithinRoot(rootAbs, target); err != nil {
		return "", err
	}
	return target, nil
}

func ensurePathWithinRoot(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("session path escapes sessions directory")
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve sessions directory symlinks: %w", err)
	}

	existing := target
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect session path: %w", err)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return fmt.Errorf("cannot resolve session path parent")
		}
		existing = parent
	}

	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("resolve session path symlinks: %w", err)
	}
	realRel, err := filepath.Rel(realRoot, realExisting)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("session path escapes sessions directory through symlink")
	}
	return nil
}

// FindSession 在指定目录内查找唯一匹配的会话。
// 优先精确路径；模糊匹配超过一个结果时返回歧义错误。
func FindSession(sessionsDir, input string) (*Session, error) {
	exactPath, err := ResolveSessionFile(sessionsDir, input)
	if err != nil {
		return nil, err
	}
	if s, loadErr := LoadSession(exactPath); loadErr == nil {
		if !s.Valid {
			return nil, fmt.Errorf("invalid session %s: %w", input, s.Error)
		}
		return s, nil
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		return nil, loadErr
	}

	query := strings.TrimSuffix(strings.TrimSuffix(filepath.Clean(filepath.FromSlash(input)), ".yaml"), ".yml")
	sessions, err := LoadAllSessions(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	var matches []*Session
	for _, sess := range sessions {
		if !sess.Valid {
			continue
		}
		relPath, relErr := filepath.Rel(sessionsDir, sess.FilePath)
		if relErr != nil {
			continue
		}
		relPath = strings.TrimSuffix(strings.TrimSuffix(relPath, ".yaml"), ".yml")
		if strings.Contains(relPath, query) || query == filepath.Base(relPath) {
			matches = append(matches, sess)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			names = append(names, GetSessionPath(sessionsDir, match))
		}
		return nil, fmt.Errorf("%w %q: %s", ErrSessionAmbiguous, input, strings.Join(names, ", "))
	}
	return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, input)
}
