package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIsStrictHostKeyDefault 测试默认（nil）时返回 false
func TestIsStrictHostKeyDefault(t *testing.T) {
	cfg := SSHConfig{}
	if cfg.IsStrictHostKey() {
		t.Error("StrictHostKey 为 nil 时应默认返回 false")
	}
}

// TestIsStrictHostKeyTrue 测试显式设为 true
func TestIsStrictHostKeyTrue(t *testing.T) {
	val := true
	cfg := SSHConfig{StrictHostKey: &val}
	if !cfg.IsStrictHostKey() {
		t.Error("StrictHostKey 为 true 时应返回 true")
	}
}

// TestIsStrictHostKeyFalse 测试显式设为 false
func TestIsStrictHostKeyFalse(t *testing.T) {
	val := false
	cfg := SSHConfig{StrictHostKey: &val}
	if cfg.IsStrictHostKey() {
		t.Error("StrictHostKey 为 false 时应返回 false")
	}
}

func TestLoadGlobalConfig(t *testing.T) {
	globalConfig = nil

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("加载默认配置失败: %v", err)
	}

	if cfg.SecureCRT.Enabled {
		t.Error("默认配置不应启用 SecureCRT")
	}
	if cfg.XShell.Enabled {
		t.Error("默认配置不应启用 XShell")
	}
	if cfg.MobaXterm.Enabled {
		t.Error("默认配置不应启用 MobaXterm")
	}
	if cfg.SSH.StrictHostKey != nil {
		t.Error("默认 StrictHostKey 应为 nil")
	}
	if cfg.SSH.IsStrictHostKey() {
		t.Error("默认 IsStrictHostKey() 应返回 false")
	}

	globalConfig = nil
}

// TestLoadGlobalConfigCached 测试配置缓存
func TestLoadGlobalConfigCached(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg1, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("第一次加载失败: %v", err)
	}

	cfg2, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("第二次加载失败: %v", err)
	}

	if cfg1 != cfg2 {
		t.Error("缓存后两次返回的配置应为同一指针")
	}

	globalConfig = nil
}

// TestLoadGlobalConfigFromFile 测试从文件加载配置
func TestLoadGlobalConfigFromFile(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 创建配置目录和文件
	configDir := filepath.Join(tmpDir, ".xsc")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	configYAML := `securecrt:
  enabled: true
  session_path: /custom/path
  password: secret123
ssh:
  strict_host_key: false
  known_hosts_file: /custom/known_hosts
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatalf("写配置文件失败: %v", err)
	}

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if !cfg.SecureCRT.Enabled {
		t.Error("SecureCRT 应为 enabled")
	}
	if cfg.SecureCRT.SessionPath != "/custom/path" {
		t.Errorf("SessionPath = %s, want /custom/path", cfg.SecureCRT.SessionPath)
	}
	if cfg.SecureCRT.Password != "" {
		t.Errorf("Password should not be loaded from YAML (yaml:\"-\"), got %s", cfg.SecureCRT.Password)
	}
	if cfg.SSH.IsStrictHostKey() {
		t.Error("显式设为 false 时 IsStrictHostKey 应返回 false")
	}
	if cfg.SSH.KnownHostsFile != "/custom/known_hosts" {
		t.Errorf("KnownHostsFile = %s, want /custom/known_hosts", cfg.SSH.KnownHostsFile)
	}

	globalConfig = nil
}

// TestLoadGlobalConfigPartialYAML 测试部分字段的 YAML 配置
func TestLoadGlobalConfigPartialYAML(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".xsc")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	// 只设置部分字段
	configYAML := `xshell:
  enabled: true
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0600); err != nil {
		t.Fatalf("写配置文件失败: %v", err)
	}

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if !cfg.XShell.Enabled {
		t.Error("XShell 应为 enabled")
	}
	// SecureCRT 应保持默认
	if cfg.SecureCRT.Enabled {
		t.Error("SecureCRT 应保持默认（未启用）")
	}

	globalConfig = nil
}

// TestLoadGlobalConfigInvalidYAML 测试无效 YAML 配置
func TestLoadGlobalConfigInvalidYAML(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".xsc")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	invalidYAML := `[invalid yaml: {`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0600); err != nil {
		t.Fatalf("写配置文件失败: %v", err)
	}

	_, err := LoadGlobalConfig()
	if err == nil {
		t.Error("无效 YAML 应返回错误")
	}

	globalConfig = nil
}

func TestSaveAndLoadGlobalConfig(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	strictHostKey := false
	cfg := &GlobalConfig{
		SecureCRT: SecureCRTConfig{
			Enabled:     true,
			SessionPath: "/test/path",
			Password:    "testpass",
		},
		SSH: SSHConfig{
			StrictHostKey:  &strictHostKey,
			KnownHostsFile: "~/.ssh/known_hosts",
		},
	}

	err := SaveGlobalConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Password 字段标记为 yaml:"-"，保存后不应持久化到文件
	// 重置缓存强制从文件重新加载
	globalConfig = nil
	loadedCfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if !loadedCfg.SecureCRT.Enabled {
		t.Error("SecureCRT.Enabled should be true")
	}
	if loadedCfg.SecureCRT.SessionPath != "/test/path" {
		t.Errorf("SessionPath = %s, want /test/path", loadedCfg.SecureCRT.SessionPath)
	}
	if loadedCfg.SecureCRT.Password != "" {
		t.Errorf("Password should not be persisted to YAML (yaml:\"-\"), got %s", loadedCfg.SecureCRT.Password)
	}
	if loadedCfg.SSH.IsStrictHostKey() {
		t.Error("SSH.StrictHostKey should be false when explicitly set")
	}

	globalConfig = nil
}

// TestSaveGlobalConfigSetsCache 测试保存后更新缓存
func TestSaveGlobalConfigSetsCache(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &GlobalConfig{
		MobaXterm: MobaXtermConfig{
			Enabled:     true,
			SessionPath: "/mobapath",
		},
	}

	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("SaveGlobalConfig 失败: %v", err)
	}

	// 保存后 globalConfig 应被设置
	if globalConfig == nil {
		t.Error("保存后 globalConfig 不应为 nil")
	}
	if !globalConfig.MobaXterm.Enabled {
		t.Error("保存后 MobaXterm 应为 enabled")
	}

	globalConfig = nil
}

func TestGetSessionsDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sessionsDir, err := GetSessionsDir()
	if err != nil {
		t.Fatalf("GetSessionsDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".xsc", "sessions")
	if sessionsDir != expected {
		t.Errorf("GetSessionsDir = %s, want %s", sessionsDir, expected)
	}

	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		t.Error("Sessions directory should be created")
	}
}

// TestGetSessionsDirPermissions 测试目录权限为 0700
func TestGetSessionsDirPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sessionsDir, err := GetSessionsDir()
	if err != nil {
		t.Fatalf("GetSessionsDir 失败: %v", err)
	}

	info, err := os.Stat(sessionsDir)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("目录权限应为 0700，实际: %o", perm)
	}
}

func TestGetConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".xsc")
	if configDir != expected {
		t.Errorf("GetConfigDir = %s, want %s", configDir, expected)
	}

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("Config directory should be created")
	}
}

func TestGetKnownHostsPath(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := GetKnownHostsPath()
	if err != nil {
		t.Fatalf("GetKnownHostsPath failed: %v", err)
	}

	if path == "" {
		t.Error("GetKnownHostsPath should return a non-empty path")
	}

	if !strings.Contains(path, "known_hosts") {
		t.Errorf("GetKnownHostsPath = %s, should contain 'known_hosts'", path)
	}

	globalConfig = nil
}

// TestGetKnownHostsPathWithConfig 测试配置中指定 known_hosts 路径
func TestGetKnownHostsPathWithConfig(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 设置自定义路径
	customPath := "/custom/known_hosts"
	globalConfig = &GlobalConfig{
		SSH: SSHConfig{
			KnownHostsFile: customPath,
		},
	}

	path, err := GetKnownHostsPath()
	if err != nil {
		t.Fatalf("GetKnownHostsPath 失败: %v", err)
	}

	if path != customPath {
		t.Errorf("期望 %s，实际: %s", customPath, path)
	}

	globalConfig = nil
}

// TestGetKnownHostsPathWithSSHDir 测试存在 ~/.ssh/known_hosts 时的路径
func TestGetKnownHostsPathWithSSHDir(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 创建 ~/.ssh/known_hosts
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	khPath := filepath.Join(sshDir, "known_hosts")
	if err := os.WriteFile(khPath, []byte(""), 0600); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	path, err := GetKnownHostsPath()
	if err != nil {
		t.Fatalf("GetKnownHostsPath 失败: %v", err)
	}

	if path != khPath {
		t.Errorf("期望 %s，实际: %s", khPath, path)
	}

	globalConfig = nil
}

// TestGetKnownHostsPathFallback 测试回退到 ~/.xsc/known_hosts
func TestGetKnownHostsPathFallback(t *testing.T) {
	globalConfig = nil
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 不创建 ~/.ssh/known_hosts

	path, err := GetKnownHostsPath()
	if err != nil {
		t.Fatalf("GetKnownHostsPath 失败: %v", err)
	}

	expected := filepath.Join(tmpDir, ".xsc", "known_hosts")
	if path != expected {
		t.Errorf("期望 %s，实际: %s", expected, path)
	}

	globalConfig = nil
}

// TestGlobalConfigStructDefaults 测试 GlobalConfig 结构体默认值
func TestGlobalConfigStructDefaults(t *testing.T) {
	cfg := GlobalConfig{}

	if cfg.SecureCRT.Enabled {
		t.Error("默认 SecureCRT.Enabled 应为 false")
	}
	if cfg.XShell.Enabled {
		t.Error("默认 XShell.Enabled 应为 false")
	}
	if cfg.MobaXterm.Enabled {
		t.Error("默认 MobaXterm.Enabled 应为 false")
	}
	if cfg.SSH.StrictHostKey != nil {
		t.Error("默认 StrictHostKey 应为 nil")
	}
}
