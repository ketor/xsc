package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// GlobalConfig 全局配置
type GlobalConfig struct {
	SecureCRT SecureCRTConfig `yaml:"securecrt,omitempty"`
	XShell    XShellConfig    `yaml:"xshell,omitempty"`
	MobaXterm MobaXtermConfig `yaml:"mobaxterm,omitempty"`
	SSH       SSHConfig       `yaml:"ssh,omitempty"`
}

// SSHConfig SSH配置
type SSHConfig struct {
	KnownHostsFile string `yaml:"known_hosts_file,omitempty"`
	StrictHostKey  *bool  `yaml:"strict_host_key,omitempty"`
	// TerminalType 指定发送给远端的终端类型。
	// 留空时自动选择：若本地 $TERM 是远端通常不支持的类型（如 xterm-ghostty），
	// 则自动替换为 xterm-256color，避免远端缺少 terminfo 条目导致显示异常。
	TerminalType string `yaml:"terminal_type,omitempty"`
}

// remoteCompatibleTerms 是远端服务器普遍支持的终端类型集合
var remoteCompatibleTerms = map[string]bool{
	"xterm":           true,
	"xterm-256color":  true,
	"xterm-color":     true,
	"vt100":           true,
	"vt220":           true,
	"screen":          true,
	"screen-256color": true,
	"tmux":            true,
	"tmux-256color":   true,
	"linux":           true,
	"ansi":            true,
}

// GetTerminalType 返回连接远端时使用的终端类型。
// 优先级：配置文件 > 本地 $TERM（若兼容）> xterm-256color
func (c SSHConfig) GetTerminalType() string {
	if c.TerminalType != "" {
		return c.TerminalType
	}
	localTerm := os.Getenv("TERM")
	if localTerm != "" && remoteCompatibleTerms[localTerm] {
		return localTerm
	}
	return "xterm-256color"
}

// IsStrictHostKey 返回是否启用严格主机密钥验证
// 默认为 false（便捷优先），仅当显式设为 true 时才启用验证
func (c SSHConfig) IsStrictHostKey() bool {
	if c.StrictHostKey == nil {
		return false // 默认禁用
	}
	return *c.StrictHostKey
}

// SecureCRTConfig SecureCRT配置
//
// Password 字段允许从 YAML 读入（便于本地 0600 配置自管），但 SaveGlobalConfig
// 永远不会把 Password 写回 YAML（见 MarshalYAML），避免保存动作把内存中的密码
// 意外落盘到配置文件。
type SecureCRTConfig struct {
	Enabled     bool   `yaml:"enabled"`
	SessionPath string `yaml:"session_path"`
	Password    string `yaml:"password,omitempty"`
}

// MarshalYAML 序列化时跳过 Password 字段
func (c SecureCRTConfig) MarshalYAML() (interface{}, error) {
	return struct {
		Enabled     bool   `yaml:"enabled"`
		SessionPath string `yaml:"session_path"`
	}{c.Enabled, c.SessionPath}, nil
}

// XShellConfig XShell配置（Password 处理同 SecureCRTConfig）
type XShellConfig struct {
	Enabled     bool   `yaml:"enabled"`
	SessionPath string `yaml:"session_path"`
	Password    string `yaml:"password,omitempty"`
}

// MarshalYAML 序列化时跳过 Password 字段
func (c XShellConfig) MarshalYAML() (interface{}, error) {
	return struct {
		Enabled     bool   `yaml:"enabled"`
		SessionPath string `yaml:"session_path"`
	}{c.Enabled, c.SessionPath}, nil
}

// MobaXtermConfig MobaXterm配置（Password 处理同 SecureCRTConfig）
type MobaXtermConfig struct {
	Enabled     bool   `yaml:"enabled"`
	SessionPath string `yaml:"session_path"`
	Password    string `yaml:"password,omitempty"`
}

// MarshalYAML 序列化时跳过 Password 字段
func (c MobaXtermConfig) MarshalYAML() (interface{}, error) {
	return struct {
		Enabled     bool   `yaml:"enabled"`
		SessionPath string `yaml:"session_path"`
	}{c.Enabled, c.SessionPath}, nil
}

var (
	globalConfig *GlobalConfig
	configMu     sync.Mutex
)

// LoadGlobalConfig 加载全局配置
func LoadGlobalConfig() (*GlobalConfig, error) {
	configMu.Lock()
	defer configMu.Unlock()

	if globalConfig != nil {
		return globalConfig, nil
	}

	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(configDir, "config.yaml")

	// 默认配置
	cfg := &GlobalConfig{
		SecureCRT: SecureCRTConfig{
			Enabled:     false,
			SessionPath: filepath.Join(configDir, "securecrt_sessions"),
			Password:    "",
		},
		XShell: XShellConfig{
			Enabled:     false,
			SessionPath: filepath.Join(configDir, "xshell_sessions"),
			Password:    "",
		},
		MobaXterm: MobaXtermConfig{
			Enabled:     false,
			SessionPath: filepath.Join(configDir, "mobaxterm_sessions"),
			Password:    "",
		},
		SSH: SSHConfig{
			KnownHostsFile: "",
			StrictHostKey:  nil, // 默认 nil → IsStrictHostKey() 返回 false
		},
	}

	// 如果配置文件存在，加载它
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}

		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(cfg); err != nil {
			return nil, err
		}
	}

	applyPasswordEnvOverrides(cfg)

	globalConfig = cfg
	return globalConfig, nil
}

// applyPasswordEnvOverrides 应用主密码环境变量覆盖。优先级（高到低）：
//  1. 源特定变量：XSC_SECURECRT_PASSWORD / XSC_XSHELL_PASSWORD / XSC_MOBAXTERM_PASSWORD
//  2. YAML 中已有的 password 字段
//  3. 通用 XSC_MASTER_PASSWORD（仅当对应源仍为空时兜底）
func applyPasswordEnvOverrides(cfg *GlobalConfig) {
	if v := os.Getenv("XSC_SECURECRT_PASSWORD"); v != "" {
		cfg.SecureCRT.Password = v
	}
	if v := os.Getenv("XSC_XSHELL_PASSWORD"); v != "" {
		cfg.XShell.Password = v
	}
	if v := os.Getenv("XSC_MOBAXTERM_PASSWORD"); v != "" {
		cfg.MobaXterm.Password = v
	}
	if master := os.Getenv("XSC_MASTER_PASSWORD"); master != "" {
		if cfg.SecureCRT.Password == "" {
			cfg.SecureCRT.Password = master
		}
		if cfg.XShell.Password == "" {
			cfg.XShell.Password = master
		}
		if cfg.MobaXterm.Password == "" {
			cfg.MobaXterm.Password = master
		}
	}
}

// SaveGlobalConfig 原子保存全局配置。
func SaveGlobalConfig(config *GlobalConfig) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(configDir, ".config-*.yaml")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temp.Close()
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, filepath.Join(configDir, "config.yaml")); err != nil {
		return err
	}
	committed = true

	configMu.Lock()
	globalConfig = config
	configMu.Unlock()
	return nil
}

// ResetForTesting 安全重置全局配置（仅供测试使用）
func ResetForTesting() {
	configMu.Lock()
	globalConfig = nil
	configMu.Unlock()
}

// GetSessionsDir 返回会话目录路径
func GetSessionsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	sessionsDir := filepath.Join(homeDir, ".xsc", "sessions")

	// 确保目录存在（使用 0700 限制访问权限，因为可能包含敏感信息）
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		return "", err
	}

	return sessionsDir, nil
}

// GetConfigDir 返回配置目录路径
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".xsc")

	// 确保目录存在（使用 0700 限制访问权限，因为可能包含密码等敏感信息）
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}

	return configDir, nil
}

// GetKnownHostsPath 返回 known_hosts 文件路径
// 优先级：配置中的路径 > ~/.ssh/known_hosts > ~/.xsc/known_hosts
func GetKnownHostsPath() (string, error) {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return "", err
	}
	if cfg.SSH.KnownHostsFile != "" {
		path := cfg.SSH.KnownHostsFile
		if path == "~" || strings.HasPrefix(path, "~/") {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return "", homeErr
			}
			path = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
		}
		return path, nil
	}

	// 检查默认的 ~/.ssh/known_hosts
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	sshKnownHosts := filepath.Join(homeDir, ".ssh", "known_hosts")
	if _, err := os.Stat(sshKnownHosts); err == nil {
		return sshKnownHosts, nil
	}

	// 使用 xssh 的 known_hosts
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "known_hosts"), nil
}
