package main

import (
	"testing"

	"github.com/ketor/xsc/internal/securecrt"
)

// TestBuildXSSHSessionFromImport_PreservesAuthMethods 回归保护 bug 2/3：
// xssh import-securecrt 必须保留完整的 AuthMethods 列表（多种认证方式按顺序），
// 否则 TUI 把导入后的 session 当成单一认证显示，丢失 publickey 等其他方法。
func TestBuildXSSHSessionFromImport_PreservesAuthMethods(t *testing.T) {
	imp := importSession{
		Name:     "test-multi-auth",
		Password: "decrypted-pwd",
		SessionData: map[string]interface{}{
			"host":      "10.0.0.1",
			"port":      22,
			"user":      "ops",
			"auth_type": "password",
			"auth_methods": []securecrt.AuthMethod{
				{Type: "password", Priority: 0, Password: "ENCRYPTED_BLOB"},
				{Type: "publickey", Priority: 1},
				{Type: "keyboard-interactive", Priority: 2},
				{Type: "gssapi", Priority: 3},
			},
			"encrypted_password": "ENCRYPTED_BLOB",
		},
	}

	got := buildXSSHSessionFromImport(imp)

	if got.Host != "10.0.0.1" || got.Port != 22 || got.User != "ops" {
		t.Errorf("base fields wrong: host=%s port=%d user=%s", got.Host, got.Port, got.User)
	}
	if len(got.AuthMethods) != 4 {
		t.Fatalf("expected 4 AuthMethods, got %d", len(got.AuthMethods))
	}
	wantTypes := []string{"password", "publickey", "keyboard-interactive", "gssapi"}
	for i, want := range wantTypes {
		if got.AuthMethods[i].Type != want {
			t.Errorf("AuthMethods[%d].Type = %q, want %q", i, got.AuthMethods[i].Type, want)
		}
		if got.AuthMethods[i].Priority != i {
			t.Errorf("AuthMethods[%d].Priority = %d, want %d", i, got.AuthMethods[i].Priority, i)
		}
	}
	// password 方法应该有解密后的明文（从 importSession.Password 来）
	if got.AuthMethods[0].Password != "decrypted-pwd" {
		t.Errorf("password AuthMethod should carry decrypted password, got %q", got.AuthMethods[0].Password)
	}
}

// TestBuildXSSHSessionFromImport_PreservesPassword 回归保护 bug 4：
// 导入时已经解密的密码必须写入到顶层 Password 字段，TUI 才能在 :pw toggle 时显示明文。
func TestBuildXSSHSessionFromImport_PreservesPassword(t *testing.T) {
	imp := importSession{
		Name:     "test-pwd",
		Password: "MyP@ssw0rd",
		SessionData: map[string]interface{}{
			"host":      "10.0.0.2",
			"port":      22,
			"user":      "admin",
			"auth_type": "password",
		},
	}

	got := buildXSSHSessionFromImport(imp)

	if got.Password != "MyP@ssw0rd" {
		t.Errorf("Password not preserved, got %q want MyP@ssw0rd", got.Password)
	}
}

// TestBuildXSSHSessionFromImport_PreservesKeyPath 验证 publickey 认证的 key 路径被保留
func TestBuildXSSHSessionFromImport_PreservesKeyPath(t *testing.T) {
	imp := importSession{
		Name: "test-key",
		SessionData: map[string]interface{}{
			"host":      "10.0.0.3",
			"port":      22,
			"user":      "deploy",
			"auth_type": "publickey",
			"key_path":  "/home/deploy/.ssh/id_ed25519",
		},
	}

	got := buildXSSHSessionFromImport(imp)

	if got.KeyPath != "/home/deploy/.ssh/id_ed25519" {
		t.Errorf("KeyPath not preserved, got %q", got.KeyPath)
	}
}

// TestBuildXSSHSessionFromImport_SingleAuth 验证单认证方式 session（无 AuthMethods）也工作
func TestBuildXSSHSessionFromImport_SingleAuth(t *testing.T) {
	imp := importSession{
		Name:     "test-single",
		Password: "simple-pwd",
		SessionData: map[string]interface{}{
			"host":      "10.0.0.4",
			"port":      22,
			"user":      "root",
			"auth_type": "password",
		},
	}

	got := buildXSSHSessionFromImport(imp)

	if got.Host != "10.0.0.4" {
		t.Errorf("host wrong: %s", got.Host)
	}
	if got.Password != "simple-pwd" {
		t.Errorf("password wrong: %q", got.Password)
	}
	if len(got.AuthMethods) != 0 {
		t.Errorf("expected no AuthMethods, got %d", len(got.AuthMethods))
	}
}
