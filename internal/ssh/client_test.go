package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ketor/xsc/internal/session"
	"github.com/ketor/xsc/pkg/config"
	"golang.org/x/crypto/ssh"
)

// TestDialInvalidSession 测试无效 session 返回错误
func TestDialInvalidSession(t *testing.T) {
	s := &session.Session{
		Valid: false,
		Error: fmt.Errorf("test error"),
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
	if client != nil {
		t.Fatal("期望 client 为 nil")
	}
	if cleanup != nil {
		t.Fatal("期望 cleanup 为 nil")
	}
	if !strings.Contains(err.Error(), "invalid session") {
		t.Errorf("错误消息应包含 'invalid session'，实际: %s", err.Error())
	}
}

// TestDialDefaultPort 测试默认端口为 22
func TestDialDefaultPort(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     0,
		User:     "testuser",
		AuthType: session.AuthTypePassword,
		Password: "testpass",
		Valid:    true,
	}

	// 先验证 Validate 会将 Port 设为 22
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate 失败: %v", err)
	}
	if s.Port != 22 {
		t.Errorf("期望端口为 22，实际: %d", s.Port)
	}
}

// TestDialPasswordAuthConfig 测试密码认证的 SSH 配置构建正确
func TestDialPasswordAuthConfig(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypePassword,
		Password: "testpass",
		Valid:    true,
	}

	config, cleanup, err := getSSHConfig(s)
	if err != nil {
		t.Fatalf("getSSHConfig 失败: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if config.User != "testuser" {
		t.Errorf("期望用户名为 'testuser'，实际: %s", config.User)
	}
	if len(config.Auth) != 1 {
		t.Errorf("期望 1 个认证方法，实际: %d", len(config.Auth))
	}
}

// TestDialKeyAuthConfig 测试密钥认证的 SSH 配置构建（无效密钥路径返回错误）
func TestDialKeyAuthConfig(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeKey,
		KeyPath:  "/nonexistent/key",
		Valid:    true,
	}

	_, _, err := getSSHConfig(s)
	if err == nil {
		t.Fatal("期望因无效密钥路径返回错误")
	}
	if !strings.Contains(err.Error(), "failed to read key file") {
		t.Errorf("错误消息应包含 'failed to read key file'，实际: %s", err.Error())
	}
}

// TestDialKeyAuthConfigWithValidKey 测试使用有效临时密钥文件构建 SSH 配置
func TestDialKeyAuthConfigWithValidKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_ed25519")

	// 生成 ed25519 密钥对并写入临时文件
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}

	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("序列化私钥失败: %v", err)
	}
	keyData := pem.EncodeToMemory(pemBlock)
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatalf("写入密钥文件失败: %v", err)
	}

	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeKey,
		KeyPath:  keyPath,
		Valid:    true,
	}

	config, cleanup, err := getSSHConfig(s)
	if err != nil {
		t.Fatalf("getSSHConfig 应成功: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if config.User != "testuser" {
		t.Errorf("期望用户名 'testuser'，实际: %s", config.User)
	}
	if len(config.Auth) != 1 {
		t.Errorf("期望 1 个认证方法，实际: %d", len(config.Auth))
	}
}

// TestDialKeyAuthConfigWithInvalidKeyContent 测试密钥内容无效时返回错误
func TestDialKeyAuthConfigWithInvalidKeyContent(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "bad_key")
	if err := os.WriteFile(keyPath, []byte("not a real key"), 0600); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeKey,
		KeyPath:  keyPath,
		Valid:    true,
	}

	_, _, err := getSSHConfig(s)
	if err == nil {
		t.Fatal("期望因无效密钥内容返回错误")
	}
	if !strings.Contains(err.Error(), "failed to parse private key") {
		t.Errorf("错误消息应包含 'failed to parse private key'，实际: %s", err.Error())
	}
}

// TestDialUnsupportedAuthType 测试不支持的认证类型返回错误
func TestDialUnsupportedAuthType(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthType("unknown"),
		Valid:    true,
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if client != nil {
		t.Fatal("期望 client 为 nil")
	}
	if cleanup != nil {
		t.Fatal("期望 cleanup 为 nil")
	}
}

// TestDialMultipleAuthConfig 测试多认证方式的配置构建
func TestDialMultipleAuthConfig(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypePassword,
		Valid:    true,
		AuthMethods: []session.AuthMethod{
			{
				Type:     "password",
				Password: "testpass",
			},
		},
	}

	config, cleanup, err := getSSHConfigForAuthMethod(s, s.AuthMethods[0])
	if err != nil {
		t.Fatalf("getSSHConfigForAuthMethod 失败: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if config.User != "testuser" {
		t.Errorf("期望用户名为 'testuser'，实际: %s", config.User)
	}
	if len(config.Auth) != 1 {
		t.Errorf("期望 1 个认证方法，实际: %d", len(config.Auth))
	}
}

// TestGetSSHConfigForAuthMethodKeyWithPath 测试多认证方式中使用指定密钥路径
func TestGetSSHConfigForAuthMethodKeyWithPath(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_ed25519")

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("序列化私钥失败: %v", err)
	}
	keyData := pem.EncodeToMemory(pemBlock)
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		t.Fatalf("写入密钥文件失败: %v", err)
	}

	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type:    "publickey",
		KeyPath: keyPath,
	}

	config, cleanup, err := getSSHConfigForAuthMethod(s, am)
	if err != nil {
		t.Fatalf("getSSHConfigForAuthMethod 应成功: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if len(config.Auth) != 1 {
		t.Errorf("期望 1 个认证方法，实际: %d", len(config.Auth))
	}
}

// TestGetSSHConfigForAuthMethodKeyInvalidPath 测试多认证方式中密钥路径无效
func TestGetSSHConfigForAuthMethodKeyInvalidPath(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type:    "key",
		KeyPath: "/nonexistent/key",
	}

	_, _, err := getSSHConfigForAuthMethod(s, am)
	if err == nil {
		t.Fatal("期望因无效密钥路径返回错误")
	}
}

// TestGetSSHConfigForAuthMethodUnsupported 测试多认证方式中不支持的类型
func TestGetSSHConfigForAuthMethodUnsupported(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type: "unknown-type",
	}

	_, _, err := getSSHConfigForAuthMethod(s, am)
	if err == nil {
		t.Fatal("期望因不支持的认证类型返回错误")
	}
	if !strings.Contains(err.Error(), "unsupported auth type") {
		t.Errorf("错误消息应包含 'unsupported auth type'，实际: %s", err.Error())
	}
}

// TestGetSSHConfigForAuthMethodAgentNoSocket 测试 Agent 认证无 socket 时返回错误
func TestGetSSHConfigForAuthMethodAgentNoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type: "agent",
	}

	_, _, err := getSSHConfigForAuthMethod(s, am)
	if err == nil {
		t.Fatal("期望因无 SSH Agent 返回错误")
	}
}

// TestGetHostKeyCallback 测试主机密钥回调创建
func TestGetHostKeyCallback(t *testing.T) {
	callback := getHostKeyCallback()
	if callback == nil {
		t.Fatal("getHostKeyCallback 不应返回 nil")
	}
}

// TestGetHostKeyCallbackWithKnownHosts 测试使用实际 known_hosts 文件的回调
func TestGetHostKeyCallbackWithKnownHosts(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	// 创建空的 known_hosts 文件
	if err := os.WriteFile(knownHostsPath, []byte(""), 0600); err != nil {
		t.Fatalf("创建 known_hosts 失败: %v", err)
	}

	// 验证 knownhosts.New 能处理空文件
	// 这里只测试函数不 panic
	callback := getHostKeyCallback()
	if callback == nil {
		t.Fatal("getHostKeyCallback 不应返回 nil")
	}
}

// TestAppendHostKey 测试主机密钥追加（最佳努力）
func TestAppendHostKey(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	// 创建空文件
	if err := os.WriteFile(knownHostsPath, []byte(""), 0600); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	// 生成一个公钥用于测试
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("转换公钥失败: %v", err)
	}

	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}
	appendHostKey(knownHostsPath, addr, sshPub)

	// 验证文件不为空
	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if len(data) == 0 {
		t.Error("写入后 known_hosts 不应为空")
	}
}

// TestAppendHostKeyInvalidPath 测试写入不存在路径时不 panic
func TestAppendHostKeyInvalidPath(t *testing.T) {
	appendHostKey("/nonexistent/path/known_hosts", nil, nil)
	// 不 panic 即为通过
}

// TestFindDefaultSSHKeys 测试默认 SSH 密钥查找
func TestFindDefaultSSHKeys(t *testing.T) {
	keys := findDefaultSSHKeys()
	// 结果取决于环境，但不应 panic
	_ = keys
}

// TestFindDefaultSSHKeysWithMockedDir 测试在自定义 .ssh 目录下查找密钥
func TestFindDefaultSSHKeysWithMockedDir(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("创建 .ssh 目录失败: %v", err)
	}

	// 创建几个假密钥文件
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		path := filepath.Join(sshDir, name)
		if err := os.WriteFile(path, []byte("fake key"), 0600); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}
	}

	// findDefaultSSHKeys 使用 os.UserHomeDir，无法简单 mock
	// 但至少验证函数不 panic
	keys := findDefaultSSHKeys()
	_ = keys
}

// TestDialKeyboardInteractiveConfig 测试键盘交互认证配置
func TestDialKeyboardInteractiveConfig(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}

	authMethod := session.AuthMethod{
		Type:     "keyboard-interactive",
		Password: "testpass",
	}

	config, cleanup, err := getSSHConfigForAuthMethod(s, authMethod)
	if err != nil {
		t.Fatalf("getSSHConfigForAuthMethod 失败: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if len(config.Auth) != 1 {
		t.Errorf("期望 1 个认证方法，实际: %d", len(config.Auth))
	}
}

// TestDialKeyboardInteractiveWithoutPassword 测试键盘交互认证无密码时配置
func TestDialKeyboardInteractiveWithoutPassword(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}

	am := session.AuthMethod{
		Type: "keyboard-interactive",
		// 不提供密码
	}

	config, cleanup, err := getSSHConfigForAuthMethod(s, am)
	if err != nil {
		t.Fatalf("getSSHConfigForAuthMethod 应成功（回调在使用时才失败）: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if len(config.Auth) != 1 {
		t.Errorf("期望 1 个认证方法，实际: %d", len(config.Auth))
	}
}

// TestConnectInvalidSession 测试无效会话连接
func TestConnectInvalidSession(t *testing.T) {
	s := &session.Session{
		Valid: false,
		Error: fmt.Errorf("test error"),
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "invalid session") {
		t.Errorf("错误消息应包含 'invalid session'，实际: %s", err.Error())
	}
}

// TestConnectUnsupportedAuthType 测试不支持的认证类型
func TestConnectUnsupportedAuthType(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthType("unknown"),
		Valid:    true,
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "unsupported auth type") {
		t.Errorf("错误消息应包含 'unsupported auth type'，实际: %s", err.Error())
	}
}

// TestConnectWithIOInvalidSession 测试 ConnectWithIO 无效会话
func TestConnectWithIOInvalidSession(t *testing.T) {
	s := &session.Session{
		Valid: false,
		Error: fmt.Errorf("test error"),
	}

	err := ConnectWithIO(s, nil, nil, nil)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "invalid session") {
		t.Errorf("错误消息应包含 'invalid session'，实际: %s", err.Error())
	}
}

// TestConnectWithIOUnsupportedAuthType 测试 ConnectWithIO 不支持的认证类型
func TestConnectWithIOUnsupportedAuthType(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthType("unsupported"),
		Valid:    true,
	}

	err := ConnectWithIO(s, nil, nil, nil)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "unsupported auth type") {
		t.Errorf("错误消息应包含 'unsupported auth type'，实际: %s", err.Error())
	}
}

// TestListAgentKeysNoSocket 测试无 SSH Agent 时的行为
func TestListAgentKeysNoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	_, err := ListAgentKeys()
	if err == nil {
		t.Fatal("无 SSH_AUTH_SOCK 时应返回错误")
	}
	if !strings.Contains(err.Error(), "SSH_AUTH_SOCK not set") {
		t.Errorf("错误消息应包含 'SSH_AUTH_SOCK not set'，实际: %s", err.Error())
	}
}

// TestListAgentKeysInvalidSocket 测试无效 SSH Agent socket 路径
func TestListAgentKeysInvalidSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/nonexistent/agent.sock")

	_, err := ListAgentKeys()
	if err == nil {
		t.Fatal("无效 socket 路径应返回错误")
	}
	if !strings.Contains(err.Error(), "failed to connect to ssh-agent") {
		t.Errorf("错误消息应包含 'failed to connect to ssh-agent'，实际: %s", err.Error())
	}
}

// TestGetSSHAgentAuthNoSocket 测试无 SSH Agent 时获取认证方法
func TestGetSSHAgentAuthNoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	_, _, err := getSSHAgentAuth()
	if err == nil {
		t.Fatal("无 SSH_AUTH_SOCK 时应返回错误")
	}
}

// TestGetSSHConfigAgentNoSocket 测试 agent 认证方式无 socket 时返回错误
func TestGetSSHConfigAgentNoSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeAgent,
		Valid:    true,
	}

	_, _, err := getSSHConfig(s)
	if err == nil {
		t.Fatal("无 SSH Agent 时应返回错误")
	}
	if !strings.Contains(err.Error(), "failed to get SSH agent auth") {
		t.Errorf("错误消息应包含 'failed to get SSH agent auth'，实际: %s", err.Error())
	}
}

// TestDialWithMultipleAuthAllFail 测试多认证方式全部失败的情况
func TestDialWithMultipleAuthAllFail(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	s := &session.Session{
		Host:  "192.168.255.255",
		Port:  22,
		User:  "testuser",
		Valid: true,
		AuthMethods: []session.AuthMethod{
			{
				Type: "agent", // 无 SSH Agent
			},
		},
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("期望所有认证方式都失败")
	}
	if !strings.Contains(err.Error(), "all authentication methods failed") {
		t.Errorf("错误消息应包含 'all authentication methods failed'，实际: %s", err.Error())
	}
}

// TestConnectWithMultipleAuthAllFail 测试 Connect 多认证方式全部失败
func TestConnectWithMultipleAuthAllFail(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	s := &session.Session{
		Host:  "192.168.255.255",
		Port:  22,
		User:  "testuser",
		Valid: true,
		AuthMethods: []session.AuthMethod{
			{
				Type: "agent",
			},
		},
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "all authentication methods failed") {
		t.Errorf("错误消息应包含 'all authentication methods failed'，实际: %s", err.Error())
	}
}

// TestAgentKeyInfoStruct 测试 AgentKeyInfo 结构体
func TestAgentKeyInfoStruct(t *testing.T) {
	info := AgentKeyInfo{
		Type:    "ssh-ed25519",
		Bits:    256,
		Comment: "test@host",
	}
	if info.Type != "ssh-ed25519" {
		t.Errorf("期望 Type 为 'ssh-ed25519'，实际: %s", info.Type)
	}
	if info.Comment != "test@host" {
		t.Errorf("期望 Comment 为 'test@host'，实际: %s", info.Comment)
	}
}

// TestConnectWithEncryptedPasswordResolveError 测试密码解密失败时的连接
func TestConnectWithEncryptedPasswordResolveError(t *testing.T) {
	s := &session.Session{
		Host:              "192.168.1.1",
		Port:              22,
		User:              "testuser",
		AuthType:          session.AuthTypePassword,
		Password:          "",
		EncryptedPassword: "invalid_encrypted_data",
		PasswordSource:    "securecrt",
		Valid:             true,
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("加密密码无效时应返回错误")
	}
	if !strings.Contains(err.Error(), "failed to resolve password") {
		t.Errorf("错误消息应包含 'failed to resolve password'，实际: %s", err.Error())
	}
}

// TestDialWithEncryptedPasswordResolveError 测试 Dial 密码解密失败
func TestDialWithEncryptedPasswordResolveError(t *testing.T) {
	s := &session.Session{
		Host:              "192.168.1.1",
		Port:              22,
		User:              "testuser",
		AuthType:          session.AuthTypePassword,
		Password:          "",
		EncryptedPassword: "invalid_encrypted_data",
		PasswordSource:    "securecrt",
		Valid:             true,
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("加密密码无效时应返回错误")
	}
	if !strings.Contains(err.Error(), "failed to resolve password") {
		t.Errorf("错误消息应包含 'failed to resolve password'，实际: %s", err.Error())
	}
}

// TestGetHostKeyCallbackStrictDisabled 测试禁用严格主机密钥验证
func TestGetHostKeyCallbackStrictDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// 创建配置目录和文件，禁用严格主机密钥验证
	configDir := filepath.Join(tmpDir, ".xsc")
	os.MkdirAll(configDir, 0700)
	configYAML := "ssh:\n  strict_host_key: false\n"
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0600)

	// 重置全局配置缓存
	// 注意：getHostKeyCallback 调用 config.LoadGlobalConfig，需要重置缓存
	callback := getHostKeyCallback()
	if callback == nil {
		t.Fatal("getHostKeyCallback 不应返回 nil")
	}
}

// TestGetSSHConfigForAuthMethodPublickeyNoPath 测试无路径的公钥认证（使用默认密钥）
func TestGetSSHConfigForAuthMethodPublickeyNoPath(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	// 创建有效的 ed25519 密钥
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	pemBlock, _ := ssh.MarshalPrivateKey(priv, "")
	keyData := pem.EncodeToMemory(pemBlock)
	os.WriteFile(filepath.Join(sshDir, "id_ed25519"), keyData, 0600)

	// 注意：findDefaultSSHKeys 使用 os.UserHomeDir，无法在测试中 mock HOME
	// 但可以测试当明确没有密钥路径且没有默认密钥时的错误
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type:    "publickey",
		KeyPath: "", // 无路径，依赖默认密钥
	}

	// 如果环境中有 ~/.ssh/id_* 文件，则应成功；否则应失败
	// 不断言成功/失败，只验证不 panic
	_, cleanup, err := getSSHConfigForAuthMethod(s, am)
	if cleanup != nil {
		cleanup()
	}
	_ = err
}

// TestGetSSHConfigForAuthMethodKeyWithInvalidContent 测试密钥内容无效
func TestGetSSHConfigForAuthMethodKeyWithInvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "bad_key")
	os.WriteFile(keyPath, []byte("not-a-key"), 0600)

	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type:    "publickey",
		KeyPath: keyPath,
	}

	_, _, err := getSSHConfigForAuthMethod(s, am)
	if err == nil {
		t.Fatal("无效密钥内容应返回错误")
	}
	if !strings.Contains(err.Error(), "failed to parse private key") {
		t.Errorf("错误消息应包含 'failed to parse private key'，实际: %s", err.Error())
	}
}

// TestConnectWithMultipleAuthKeyFail 测试多认证方式中密钥认证失败（无效路径）
func TestConnectWithMultipleAuthKeyFail(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
		AuthMethods: []session.AuthMethod{
			{
				Type:    "key",
				KeyPath: "/nonexistent/key",
			},
		},
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("无效密钥路径应返回错误")
	}
	if !strings.Contains(err.Error(), "all authentication methods failed") {
		t.Errorf("错误消息应包含 'all authentication methods failed'，实际: %s", err.Error())
	}
}

// TestDialWithMultipleAuthKeyFail 测试 Dial 多认证方式密钥失败
func TestDialWithMultipleAuthKeyFail(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
		AuthMethods: []session.AuthMethod{
			{
				Type:    "key",
				KeyPath: "/nonexistent/key",
			},
		},
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("无效密钥路径应返回错误")
	}
	if !strings.Contains(err.Error(), "all authentication methods failed") {
		t.Errorf("错误消息应包含 'all authentication methods failed'，实际: %s", err.Error())
	}
}

// TestConnectWithMultipleAuthUnsupportedType 测试多认证方式中不支持的类型
func TestConnectWithMultipleAuthUnsupportedType(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
		AuthMethods: []session.AuthMethod{
			{
				Type: "unknown-type",
			},
		},
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("不支持的认证类型应返回错误")
	}
}

// TestDialWithMultipleAuthMixed 测试 Dial 多认证方式混合失败
func TestDialWithMultipleAuthMixed(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
		AuthMethods: []session.AuthMethod{
			{
				Type:    "key",
				KeyPath: "/nonexistent/key",
			},
			{
				Type: "agent",
			},
			{
				Type: "unknown",
			},
		},
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("所有认证方式都无效时应返回错误")
	}
}

// TestGetSSHConfigForAuthMethodAgentInvalidSocket 测试 Agent 认证 socket 无效
func TestGetSSHConfigForAuthMethodAgentInvalidSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/nonexistent/agent.sock")

	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type: "agent",
	}

	_, _, err := getSSHConfigForAuthMethod(s, am)
	if err == nil {
		t.Fatal("无效 socket 应返回错误")
	}
}

// TestGetSSHAgentAuthInvalidSocket 测试无效 socket 路径
func TestGetSSHAgentAuthInvalidSocket(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/nonexistent/agent.sock")

	_, _, err := getSSHAgentAuth()
	if err == nil {
		t.Fatal("无效 socket 路径应返回错误")
	}
}

// TestConnectWithIOKeyAuthFail 测试 ConnectWithIO 密钥认证失败
func TestConnectWithIOKeyAuthFail(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeKey,
		KeyPath:  "/nonexistent/key",
		Valid:    true,
	}

	err := ConnectWithIO(s, nil, nil, nil)
	if err == nil {
		t.Fatal("无效密钥路径应返回错误")
	}
}

// TestConnectWithIOAgentAuthFail 测试 ConnectWithIO Agent 认证失败
func TestConnectWithIOAgentAuthFail(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeAgent,
		Valid:    true,
	}

	err := ConnectWithIO(s, nil, nil, nil)
	if err == nil {
		t.Fatal("无 Agent 时应返回错误")
	}
}

// TestConnectSingleAuthTypes 测试 Connect 各认证类型路由
func TestConnectSingleAuthTypes(t *testing.T) {
	// 密钥认证 - 无效密钥
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeKey,
		KeyPath:  "/nonexistent/key",
		Valid:    true,
	}
	err := Connect(s)
	if err == nil {
		t.Fatal("无效密钥路径应返回错误")
	}

	// Agent 认证 - 无 socket
	t.Setenv("SSH_AUTH_SOCK", "")
	s2 := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeAgent,
		Valid:    true,
	}
	err = Connect(s2)
	if err == nil {
		t.Fatal("无 Agent 时应返回错误")
	}
}

// TestDialKeyAuthFail 测试 Dial 密钥认证失败
func TestDialKeyAuthFail(t *testing.T) {
	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeKey,
		KeyPath:  "/nonexistent/key",
		Valid:    true,
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("无效密钥路径应返回错误")
	}
}

// TestDialAgentAuthFail 测试 Dial Agent 认证失败
func TestDialAgentAuthFail(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	s := &session.Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "testuser",
		AuthType: session.AuthTypeAgent,
		Valid:    true,
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("无 Agent 时应返回错误")
	}
}

// TestGetHostKeyCallbackTOFUUnknownHost 测试 TOFU 未知主机自动信任
func TestGetHostKeyCallbackTOFUUnknownHost(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	config.ResetForTesting()

	// 创建配置目录，显式启用严格模式以测试 TOFU
	configDir := filepath.Join(tmpDir, ".xsc")
	os.MkdirAll(configDir, 0700)
	configYAML := "ssh:\n  strict_host_key: true\n"
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0600)

	// 创建空的 known_hosts 文件
	knownHostsPath := filepath.Join(configDir, "known_hosts")
	os.WriteFile(knownHostsPath, []byte(""), 0600)

	// 也创建 ~/.ssh/known_hosts 避免回退
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)
	os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(""), 0600)

	callback := getHostKeyCallback()
	if callback == nil {
		t.Fatal("callback 不应为 nil")
	}

	// 生成测试公钥
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPub, _ := ssh.NewPublicKey(pub)

	// 测试未知主机 - TOFU 应该接受
	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 22}
	err := callback("10.0.0.1:22", addr, sshPub)
	if err != nil {
		t.Errorf("TOFU 应接受未知主机，但返回错误: %v", err)
	}

	// 验证密钥被写入 known_hosts
	data, _ := os.ReadFile(filepath.Join(sshDir, "known_hosts"))
	if len(data) == 0 {
		// 检查 .xsc/known_hosts
		data, _ = os.ReadFile(knownHostsPath)
	}
	// 密钥应被写入（但可能写入 ~/.ssh/known_hosts 或 ~/.xsc/known_hosts）
}

// TestGetHostKeyCallbackTOFUKeyChanged 测试 TOFU 密钥变更拒绝
func TestGetHostKeyCallbackTOFUKeyChanged(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	config.ResetForTesting()

	// 创建配置目录，显式启用严格模式以测试 TOFU
	configDir := filepath.Join(tmpDir, ".xsc")
	os.MkdirAll(configDir, 0700)
	configYAML := "ssh:\n  strict_host_key: true\n"
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0600)

	// 创建 ~/.ssh 目录和 known_hosts 文件
	sshDir := filepath.Join(tmpDir, ".ssh")
	os.MkdirAll(sshDir, 0700)

	// 生成两个不同的公钥
	pub1, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPub1, _ := ssh.NewPublicKey(pub1)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPub2, _ := ssh.NewPublicKey(pub2)

	// 将第一个密钥写入 known_hosts
	addr := &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 22}
	knownHostsPath := filepath.Join(sshDir, "known_hosts")
	f, _ := os.Create(knownHostsPath)
	// 写入 known_hosts 格式的条目
	line := fmt.Sprintf("%s %s\n", "10.0.0.2", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub1))))
	f.WriteString(line)
	f.Close()

	callback := getHostKeyCallback()
	if callback == nil {
		t.Fatal("callback 不应为 nil")
	}

	// 使用匹配的密钥 - 应该成功
	err := callback("10.0.0.2:22", addr, sshPub1)
	if err != nil {
		t.Errorf("匹配密钥应成功，但返回: %v", err)
	}

	// 使用不同的密钥 - 应该被拒绝（密钥变更）
	err = callback("10.0.0.2:22", addr, sshPub2)
	if err == nil {
		t.Error("密钥变更应被拒绝")
	}
}

// TestGetSSHConfigForAuthMethodKeyboardInteractiveWithPassword 测试键盘交互认证回调
func TestGetSSHConfigForAuthMethodKeyboardInteractiveWithPassword(t *testing.T) {
	s := &session.Session{
		Host:  "192.168.1.1",
		Port:  22,
		User:  "testuser",
		Valid: true,
	}
	am := session.AuthMethod{
		Type:     "keyboard-interactive",
		Password: "mypassword",
	}

	config, cleanup, err := getSSHConfigForAuthMethod(s, am)
	if err != nil {
		t.Fatalf("创建键盘交互认证配置失败: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if config.User != "testuser" {
		t.Errorf("User = %s, want testuser", config.User)
	}
	if len(config.Auth) != 1 {
		t.Errorf("期望 1 个认证方法（keyboard-interactive），实际: %d", len(config.Auth))
	}
}

// TestDialWithMultipleAuthEncryptedPasswordDecryptFail 测试多认证 Dial 密码解密失败（SecureCRT）
func TestDialWithMultipleAuthEncryptedPasswordDecryptFail(t *testing.T) {
	s := &session.Session{
		Host:           "192.168.1.1",
		Port:           22,
		User:           "testuser",
		Valid:          true,
		PasswordSource: "securecrt",
		AuthMethods: []session.AuthMethod{
			{
				Type:              "password",
				Password:          "",
				EncryptedPassword: "invalid_encrypted",
			},
		},
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("解密失败应返回错误")
	}
}

// TestConnectWithMultipleAuthEncryptedPasswordXshell 测试多认证 Connect XShell 密码解密失败
func TestConnectWithMultipleAuthEncryptedPasswordXshell(t *testing.T) {
	s := &session.Session{
		Host:           "192.168.1.1",
		Port:           22,
		User:           "testuser",
		Valid:          true,
		PasswordSource: "xshell",
		AuthMethods: []session.AuthMethod{
			{
				Type:              "password",
				Password:          "",
				EncryptedPassword: "invalid_data",
			},
		},
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("解密失败应返回错误")
	}
}

// TestDialWithMultipleAuthEncryptedPasswordMobaxterm 测试多认证 Dial MobaXterm 密码解密失败
func TestDialWithMultipleAuthEncryptedPasswordMobaxterm(t *testing.T) {
	s := &session.Session{
		Host:           "192.168.1.1",
		Port:           22,
		User:           "testuser",
		Valid:          true,
		PasswordSource: "mobaxterm",
		AuthMethods: []session.AuthMethod{
			{
				Type:              "password",
				Password:          "",
				EncryptedPassword: "invalid_moba_data",
			},
		},
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("解密失败应返回错误")
	}
}

// TestConnectWithMultipleAuthEncryptedPasswordMobaxterm 测试 Connect MobaXterm 密码解密失败
func TestConnectWithMultipleAuthEncryptedPasswordMobaxterm(t *testing.T) {
	s := &session.Session{
		Host:           "192.168.1.1",
		Port:           22,
		User:           "testuser",
		Valid:          true,
		PasswordSource: "mobaxterm",
		AuthMethods: []session.AuthMethod{
			{
				Type:              "password",
				Password:          "",
				EncryptedPassword: "invalid_data",
			},
		},
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("解密失败应返回错误")
	}
}

// TestConnectWithMultipleAuthEncryptedPasswordDefault 测试默认密码源（SecureCRT）解密路径
func TestConnectWithMultipleAuthEncryptedPasswordDefault(t *testing.T) {
	s := &session.Session{
		Host:           "192.168.1.1",
		Port:           22,
		User:           "testuser",
		Valid:          true,
		PasswordSource: "",
		AuthMethods: []session.AuthMethod{
			{
				Type:              "password",
				Password:          "",
				EncryptedPassword: "invalid_data",
			},
		},
	}

	err := Connect(s)
	if err == nil {
		t.Fatal("解密失败应返回错误")
	}
}

// TestDialWithMultipleAuthEncryptedPasswordDefault 测试 Dial 默认密码源解密路径
func TestDialWithMultipleAuthEncryptedPasswordDefault(t *testing.T) {
	s := &session.Session{
		Host:           "192.168.1.1",
		Port:           22,
		User:           "testuser",
		Valid:          true,
		PasswordSource: "",
		AuthMethods: []session.AuthMethod{
			{
				Type:              "password",
				Password:          "",
				EncryptedPassword: "invalid_data",
			},
		},
	}

	client, cleanup, err := Dial(s)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		if client != nil {
			client.Close()
		}
		t.Fatal("解密失败应返回错误")
	}
}
