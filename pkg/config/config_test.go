package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGlobalConfig(t *testing.T) {
	// 测试配置文件不存在的情况
	cfg, err := LoadGlobalConfig()
	// 应该返回错误，因为默认配置文件不存在
	if err == nil {
		// 如果没有错误，检查是否返回了默认配置
		if !cfg.SecureCRT.Enabled {
			t.Error("默认配置应该启用 SecureCRT")
		}
	}
}

func TestSaveAndLoadGlobalConfig(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "xsc-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 设置临时配置目录
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// 创建配置
	cfg := &GlobalConfig{
		SecureCRT: SecureCRTConfig{
			Enabled:     true,
			SessionPath: "/test/path",
			Password:    "testpass",
		},
		SSH: SSHConfig{
			StrictHostKey:  false,
			KnownHostsFile: "~/.ssh/known_hosts",
		},
	}

	// 保存配置
	err = SaveGlobalConfig(cfg)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// 重新加载配置
	loadedCfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证配置值
	if !loadedCfg.SecureCRT.Enabled {
		t.Error("SecureCRT.Enabled should be true")
	}
	if loadedCfg.SecureCRT.SessionPath != "/test/path" {
		t.Errorf("SessionPath = %s, want /test/path", loadedCfg.SecureCRT.SessionPath)
	}
	if loadedCfg.SecureCRT.Password != "testpass" {
		t.Errorf("Password = %s, want testpass", loadedCfg.SecureCRT.Password)
	}
	if loadedCfg.SSH.StrictHostKey {
		t.Error("SSH.StrictHostKey should be false")
	}
}

func TestGetSessionsDir(t *testing.T) {
	// 设置临时 home 目录
	tmpDir, err := os.MkdirTemp("", "xsc-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	sessionsDir, err := GetSessionsDir()
	if err != nil {
		t.Fatalf("GetSessionsDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".xsc", "sessions")
	if sessionsDir != expected {
		t.Errorf("GetSessionsDir = %s, want %s", sessionsDir, expected)
	}

	// 验证目录是否被创建
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		t.Error("Sessions directory should be created")
	}
}

func TestGetConfigDir(t *testing.T) {
	// 设置临时 home 目录
	tmpDir, err := os.MkdirTemp("", "xsc-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	configDir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".xsc")
	if configDir != expected {
		t.Errorf("GetConfigDir = %s, want %s", configDir, expected)
	}

	// 验证目录是否被创建
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("Config directory should be created")
	}
}

func TestGetKnownHostsPath(t *testing.T) {
	// 测试函数能正常返回路径
	path, err := GetKnownHostsPath()
	if err != nil {
		t.Fatalf("GetKnownHostsPath failed: %v", err)
	}

	// 验证返回的路径不为空
	if path == "" {
		t.Error("GetKnownHostsPath should return a non-empty path")
	}

	// 验证路径包含 known_hosts
	if !contains(path, "known_hosts") {
		t.Errorf("GetKnownHostsPath = %s, should contain 'known_hosts'", path)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr || len(s) > len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
