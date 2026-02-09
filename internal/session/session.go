package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/user/xsc/internal/securecrt"
	"gopkg.in/yaml.v3"
)

// AuthType 定义认证类型
type AuthType string

const (
	AuthTypePassword AuthType = "password"
	AuthTypeKey      AuthType = "key"
	AuthTypeAgent    AuthType = "agent"
)

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
	AuthMethods []AuthMethod `yaml:"auth_methods,omitempty"` // 认证方法列表（按优先级）

	// 内部字段
	FilePath          string `yaml:"-"`
	Name              string `yaml:"-"`
	Valid             bool   `yaml:"-"`
	Error             error  `yaml:"-"`
	EncryptedPassword string `yaml:"-"` // SecureCRT 加密密码（延迟解密）
	MasterPassword    string `yaml:"-"` // SecureCRT 主密码（用于解密）
}

// Validate 验证会话配置是否有效
func (s *Session) Validate() error {
	if s.Host == "" {
		return fmt.Errorf("host is required")
	}
	if s.Port == 0 {
		s.Port = 22
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

	switch s.AuthType {
	case AuthTypePassword:
		if s.Password == "" {
			return fmt.Errorf("password is required when auth_type is 'password'")
		}
	case AuthTypeKey:
		if s.KeyPath == "" {
			return fmt.Errorf("key_path is required when auth_type is 'key'")
		}
		// 扩展 ~ 到 home 目录
		if s.KeyPath[0] == '~' {
			home, err := os.UserHomeDir()
			if err == nil {
				s.KeyPath = filepath.Join(home, s.KeyPath[1:])
			}
		}
		// 检查私钥文件是否存在
		if _, err := os.Stat(s.KeyPath); os.IsNotExist(err) {
			return fmt.Errorf("key file not found: %s", s.KeyPath)
		}
	case AuthTypeAgent:
		// Agent 认证不需要额外配置
	default:
		return fmt.Errorf("invalid auth_type: %s", s.AuthType)
	}

	return nil
}

// ResolvePassword 延迟解密密码（用于 SecureCRT 会话）
// 如果密码已解密或不需要解密，直接返回
func (s *Session) ResolvePassword() error {
	if s.Password != "" || s.EncryptedPassword == "" {
		return nil
	}
	if s.MasterPassword == "" {
		return fmt.Errorf("master password not set for decryption")
	}
	decrypted, err := securecrt.DecryptPassword(s.EncryptedPassword, s.MasterPassword)
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
	if err := yaml.Unmarshal(data, &session); err != nil {
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
