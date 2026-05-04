package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionValidate(t *testing.T) {
	tests := []struct {
		name    string
		session Session
		wantErr bool
	}{
		{
			name: "valid password auth",
			session: Session{
				Host:     "192.168.1.1",
				Port:     22,
				User:     "root",
				AuthType: AuthTypePassword,
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "missing host",
			session: Session{
				Port:     22,
				User:     "root",
				AuthType: AuthTypePassword,
				Password: "secret",
			},
			wantErr: true,
		},
		{
			name: "password auth without password - allows interactive input",
			session: Session{
				Host:     "192.168.1.1",
				Port:     22,
				User:     "root",
				AuthType: AuthTypePassword,
			},
			wantErr: false,
		},
		{
			name: "agent auth - no extra required",
			session: Session{
				Host:     "192.168.1.1",
				Port:     22,
				User:     "root",
				AuthType: AuthTypeAgent,
			},
			wantErr: false,
		},
		{
			name: "default port and user",
			session: Session{
				Host:     "192.168.1.1",
				AuthType: AuthTypeAgent,
			},
			wantErr: false,
		},
		{
			name: "default auth type",
			session: Session{
				Host: "192.168.1.1",
				User: "root",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.session.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSessionValidateDefaultPort 测试端口默认值
func TestSessionValidateDefaultPort(t *testing.T) {
	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthTypeAgent,
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate 失败: %v", err)
	}
	if s.Port != 22 {
		t.Errorf("期望端口 22，实际: %d", s.Port)
	}
}

// TestSessionValidateDefaultUser 测试用户名默认值
func TestSessionValidateDefaultUser(t *testing.T) {
	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthTypeAgent,
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate 失败: %v", err)
	}
	// 用户名应为 $USER 或 "root"
	if s.User == "" {
		t.Error("User 不应为空")
	}
}

// TestSessionValidateDefaultAuthType 测试默认认证类型
func TestSessionValidateDefaultAuthType(t *testing.T) {
	s := Session{
		Host: "192.168.1.1",
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate 失败: %v", err)
	}
	if s.AuthType != AuthTypeAgent {
		t.Errorf("期望默认 AuthType 为 agent，实际: %s", s.AuthType)
	}
}

// TestSessionValidateInvalidAuthType 测试无效的认证类型
func TestSessionValidateInvalidAuthType(t *testing.T) {
	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthType("invalid"),
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("期望无效认证类型返回错误")
	}
}

// TestSessionValidateKeyAuthMissingKeyPath 验证 key 认证不强制要求 key_path
//
// 设计说明：key_path 为空时，连接层（ssh/client.go findDefaultSSHKeys）会自动回退到
// ~/.ssh/ 下的默认密钥。这与 OpenSSH 的 `ssh user@host` 默认行为一致，也是 SecureCRT
// publickey 认证（不指定 Identity Filename）的常见情况。Validate 不应在此报错。
func TestSessionValidateKeyAuthMissingKeyPath(t *testing.T) {
	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthTypeKey,
	}
	if err := s.Validate(); err != nil {
		t.Errorf("key_path 为空时不应报错（连接层回退默认密钥），实际: %v", err)
	}
}

// TestSessionValidatePublicKeyAuthMissingKeyPathAccepted 验证 publickey 别名同样不强制
// key_path（保护从 SecureCRT 导入的 publickey 会话）
func TestSessionValidatePublicKeyAuthMissingKeyPathAccepted(t *testing.T) {
	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthType("publickey"),
	}
	if err := s.Validate(); err != nil {
		t.Errorf("publickey 缺 key_path 时不应报错（连接层回退默认密钥），实际: %v", err)
	}
	if s.AuthType != AuthTypeKey {
		t.Errorf("publickey 应规范化为 key，实际: %q", s.AuthType)
	}
}

// TestSessionValidateKeyAuthNonexistentFile 测试密钥文件不存在
func TestSessionValidateKeyAuthNonexistentFile(t *testing.T) {
	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthTypeKey,
		KeyPath:  "/nonexistent/key",
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("期望密钥文件不存在时返回错误")
	}
}

// TestSessionValidateAuthTypeAliases 验证 SSH 标准/SecureCRT 常见别名被规范化为 key
//
// 来源说明：
//   - "publickey" — SSH 协议标准术语，OpenSSH/SecureCRT 都使用
//   - "rsa"/"dsa"/"ecdsa"/"ed25519" — 具体密钥算法名，SecureCRT 的 Authentication 字段
//     和某些客户端把它们当作 publickey 的同义词（见 internal/securecrt/parser.go
//     parseAuthMethods 的实际处理）
func TestSessionValidateAuthTypeAliases(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0600); err != nil {
		t.Fatalf("创建临时密钥文件失败: %v", err)
	}

	aliases := []string{"publickey", "rsa", "dsa", "ecdsa", "ed25519"}
	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			s := Session{
				Host:     "192.168.1.1",
				AuthType: AuthType(alias),
				KeyPath:  keyPath,
			}
			if err := s.Validate(); err != nil {
				t.Errorf("auth_type=%q 应被接受为 key 的别名，Validate 返回: %v", alias, err)
			}
			if s.AuthType != AuthTypeKey {
				t.Errorf("auth_type=%q 应规范化为 %q，实际 %q", alias, AuthTypeKey, s.AuthType)
			}
		})
	}
}

// TestSessionValidatePublicKeyAuthAccepted 验证 auth_type: publickey 被接受为 key 的别名
// 回归保护：SecureCRT 导出 / SSH 协议都用 "publickey" 作为标准术语，
// Validate 不应该把它判为 invalid 而连带让整个 session 显示 [invalid]。
func TestSessionValidatePublicKeyAuthAccepted(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0600); err != nil {
		t.Fatalf("创建临时密钥文件失败: %v", err)
	}

	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthType("publickey"),
		KeyPath:  keyPath,
	}
	if err := s.Validate(); err != nil {
		t.Errorf("auth_type=publickey 应被接受为 key 的别名，但 Validate 返回: %v", err)
	}
	// 期望规范化为 "key" 以便上层一致处理
	if s.AuthType != AuthTypeKey {
		t.Errorf("期望 publickey 规范化为 %q，实际 %q", AuthTypeKey, s.AuthType)
	}
}

// TestSessionValidateKeyAuthWithValidFile 测试密钥认证使用有效文件
func TestSessionValidateKeyAuthWithValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0600); err != nil {
		t.Fatalf("创建临时密钥文件失败: %v", err)
	}

	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthTypeKey,
		KeyPath:  keyPath,
	}
	if err := s.Validate(); err != nil {
		t.Errorf("使用有效密钥文件路径时 Validate 不应失败: %v", err)
	}
}

// TestSessionValidateKeyPathTildeExpansion 测试路径中 ~ 的展开
func TestSessionValidateKeyPathTildeExpansion(t *testing.T) {
	s := Session{
		Host:     "192.168.1.1",
		AuthType: AuthTypeKey,
		KeyPath:  "~/nonexistent_key",
	}
	// Validate 会展开 ~ 但文件不存在所以仍报错
	err := s.Validate()
	if err == nil {
		t.Fatal("期望因文件不存在返回错误")
	}
	// 验证路径已被展开（不再以 ~ 开头）
	if s.KeyPath[0] == '~' {
		t.Error("~ 应已被展开")
	}
}

func TestSessionDisplayName(t *testing.T) {
	s := &Session{
		Name: "test-session",
		Host: "192.168.1.1",
	}
	if got := s.DisplayName(); got != "test-session" {
		t.Errorf("DisplayName() = %v, want test-session", got)
	}

	s2 := &Session{
		Host: "192.168.1.1",
	}
	if got := s2.DisplayName(); got != "192.168.1.1" {
		t.Errorf("DisplayName() = %v, want 192.168.1.1", got)
	}
}

// TestSessionDisplayNameEmpty 测试名称和 Host 都为空的情况
func TestSessionDisplayNameEmpty(t *testing.T) {
	s := &Session{}
	if got := s.DisplayName(); got != "" {
		t.Errorf("DisplayName() = %q, want empty string", got)
	}
}

func TestLoadAndSaveSession(t *testing.T) {
	tmpDir := t.TempDir()

	session := &Session{
		Host:        "192.168.1.100",
		Port:        22,
		User:        "root",
		AuthType:    AuthTypePassword,
		Password:    "testpass",
		Description: "Test session",
		AuthMethods: []AuthMethod{
			{Type: "password", Priority: 0},
		},
	}

	sessionPath := filepath.Join(tmpDir, "test-session.yaml")
	err := SaveSession(session, sessionPath)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Error("Session file should exist")
	}

	loadedSession, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	if loadedSession.Host != "192.168.1.100" {
		t.Errorf("Host = %s, want 192.168.1.100", loadedSession.Host)
	}
	if loadedSession.Port != 22 {
		t.Errorf("Port = %d, want 22", loadedSession.Port)
	}
	if loadedSession.User != "root" {
		t.Errorf("User = %s, want root", loadedSession.User)
	}
	if loadedSession.Password != "testpass" {
		t.Errorf("Password = %s, want testpass", loadedSession.Password)
	}
	if loadedSession.Name != "test-session" {
		t.Errorf("Name = %s, want test-session", loadedSession.Name)
	}
}

// TestSaveSessionCreatesDirectory 测试保存时自动创建目录
func TestSaveSessionCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "a", "b", "c")
	sessionPath := filepath.Join(nestedDir, "session.yaml")

	s := &Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		AuthType: AuthTypePassword,
		Password: "test",
	}

	if err := SaveSession(s, sessionPath); err != nil {
		t.Fatalf("SaveSession 应自动创建目录: %v", err)
	}
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Error("保存后文件应存在")
	}
}

// TestSaveSessionFilePermissions 测试保存文件权限为 0600
func TestSaveSessionFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	sessionPath := filepath.Join(tmpDir, "perms.yaml")

	s := &Session{
		Host:     "192.168.1.1",
		Port:     22,
		AuthType: AuthTypeAgent,
	}

	if err := SaveSession(s, sessionPath); err != nil {
		t.Fatalf("SaveSession 失败: %v", err)
	}

	info, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("期望文件权限 0600，实际: %o", perm)
	}
}

func TestLoadSessionInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	invalidYAML := []byte("invalid: yaml: content: [")
	sessionPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(sessionPath, invalidYAML, 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	session, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatalf("LoadSession should not return error for invalid YAML: %v", err)
	}

	if session.Valid {
		t.Error("Session should be marked as invalid for invalid YAML")
	}
	if session.Error == nil {
		t.Error("Session should have error for invalid YAML")
	}
}

// TestLoadSessionNonexistentFile 测试加载不存在的文件
func TestLoadSessionNonexistentFile(t *testing.T) {
	_, err := LoadSession("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("期望加载不存在文件时返回错误")
	}
}

// TestLoadSessionValidationFail 测试加载有效 YAML 但验证失败的会话
func TestLoadSessionValidationFail(t *testing.T) {
	tmpDir := t.TempDir()
	// 有效 YAML 但缺少 host 字段
	yamlContent := []byte("port: 22\nuser: root\nauth_type: password\n")
	sessionPath := filepath.Join(tmpDir, "nohost.yaml")
	if err := os.WriteFile(sessionPath, yamlContent, 0600); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	s, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatalf("LoadSession 不应返回 error: %v", err)
	}
	if s.Valid {
		t.Error("缺少 host 的会话不应标记为有效")
	}
	if s.Error == nil {
		t.Error("缺少 host 的会话应有错误")
	}
}

func TestAuthMethodTypes(t *testing.T) {
	methods := []AuthMethod{
		{Type: "password", Priority: 0, Password: "test"},
		{Type: "publickey", Priority: 1, KeyPath: "/path/to/key"},
		{Type: "agent", Priority: 2},
	}

	session := &Session{
		Host:        "192.168.1.1",
		AuthType:    AuthTypeAgent,
		AuthMethods: methods,
	}

	if err := session.Validate(); err != nil {
		t.Errorf("Validate() with AuthMethods should not fail: %v", err)
	}
}

// TestResolvePasswordNoEncryptedPassword 测试无加密密码时的行为
func TestResolvePasswordNoEncryptedPassword(t *testing.T) {
	s := &Session{Password: "already-set"}
	if err := s.ResolvePassword(); err != nil {
		t.Errorf("已有明文密码时不应报错: %v", err)
	}
	if s.Password != "already-set" {
		t.Error("明文密码不应被修改")
	}
}

// TestResolvePasswordEmptyEncrypted 测试加密密码为空时的行为
func TestResolvePasswordEmptyEncrypted(t *testing.T) {
	s := &Session{EncryptedPassword: ""}
	if err := s.ResolvePassword(); err != nil {
		t.Errorf("加密密码为空时不应报错: %v", err)
	}
}

// TestResolvePasswordNoMasterPassword 测试缺少主密码时返回错误
func TestResolvePasswordNoMasterPassword(t *testing.T) {
	s := &Session{
		EncryptedPassword: "encrypted-data",
		MasterPassword:    "",
	}
	err := s.ResolvePassword()
	if err == nil {
		t.Fatal("缺少主密码时应返回错误")
	}
}

// TestResolvePasswordUnknownSource 测试未知密码来源返回错误
func TestResolvePasswordUnknownSource(t *testing.T) {
	s := &Session{
		EncryptedPassword: "encrypted-data",
		MasterPassword:    "master",
		PasswordSource:    "unknown-source",
	}
	err := s.ResolvePassword()
	if err == nil {
		t.Fatal("未知密码来源时应返回错误")
	}
}

// TestFindSessionInDirectory 测试在目录中查找会话
func TestFindSessionInDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试会话
	s := &Session{
		Host:     "192.168.1.1",
		Port:     22,
		User:     "root",
		AuthType: AuthTypeAgent,
	}
	sessionPath := filepath.Join(tmpDir, "myserver.yaml")
	if err := SaveSession(s, sessionPath); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	// 精确匹配
	found, err := FindSession(tmpDir, "myserver")
	if err != nil {
		t.Fatalf("FindSession 精确匹配失败: %v", err)
	}
	if found.Host != "192.168.1.1" {
		t.Errorf("期望 Host 为 192.168.1.1，实际: %s", found.Host)
	}
}

// TestFindSessionNotFound 测试查找不存在的会话
func TestFindSessionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := FindSession(tmpDir, "nonexistent")
	if err == nil {
		t.Fatal("期望查找不存在的会话时返回错误")
	}
}

// TestLoadAllSessionsEmpty 测试加载空目录
func TestLoadAllSessionsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	sessions, err := LoadAllSessions(tmpDir)
	if err != nil {
		t.Fatalf("LoadAllSessions 失败: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("空目录应返回 0 个会话，实际: %d", len(sessions))
	}
}

// TestLoadAllSessionsMultiple 测试加载多个会话
func TestLoadAllSessionsMultiple(t *testing.T) {
	tmpDir := t.TempDir()

	for _, name := range []string{"srv1.yaml", "srv2.yaml"} {
		s := &Session{
			Host:     "192.168.1.1",
			Port:     22,
			AuthType: AuthTypeAgent,
		}
		if err := SaveSession(s, filepath.Join(tmpDir, name)); err != nil {
			t.Fatalf("保存会话失败: %v", err)
		}
	}

	// 创建一个非 yaml 文件（应被忽略）
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	sessions, err := LoadAllSessions(tmpDir)
	if err != nil {
		t.Fatalf("LoadAllSessions 失败: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("期望 2 个会话，实际: %d", len(sessions))
	}
}

// TestLoadAllSessionsWithSubdirs 测试递归加载子目录中的会话
func TestLoadAllSessionsWithSubdirs(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "group1")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	s := &Session{
		Host:     "10.0.0.1",
		Port:     22,
		AuthType: AuthTypeAgent,
	}
	if err := SaveSession(s, filepath.Join(subDir, "nested.yaml")); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	sessions, err := LoadAllSessions(tmpDir)
	if err != nil {
		t.Fatalf("LoadAllSessions 失败: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("期望 1 个会话，实际: %d", len(sessions))
	}
}

// TestLoadSessionsTree 测试树形加载
func TestLoadSessionsTree(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建嵌套结构
	subDir := filepath.Join(tmpDir, "production")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	s := &Session{
		Host:     "10.0.0.1",
		Port:     22,
		AuthType: AuthTypeAgent,
	}
	if err := SaveSession(s, filepath.Join(subDir, "web.yaml")); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}
	if err := SaveSession(s, filepath.Join(tmpDir, "local.yaml")); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	tree, err := LoadSessionsTree(tmpDir)
	if err != nil {
		t.Fatalf("LoadSessionsTree 失败: %v", err)
	}
	if tree == nil {
		t.Fatal("LoadSessionsTree 不应返回 nil")
	}
	if tree.Name != "sessions" {
		t.Errorf("根节点名称应为 'sessions'，实际: %s", tree.Name)
	}
	if len(tree.Children) != 2 {
		t.Errorf("期望 2 个子节点（1 目录 + 1 文件），实际: %d", len(tree.Children))
	}
}
