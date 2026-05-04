package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ketor/xsc/internal/securecrt"
	"github.com/ketor/xsc/internal/session"
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

// TestImportRoundTrip_PreservesFieldsThroughDisk 端到端 round-trip 防回归测试：
//
//	import_data → buildXSSHSessionFromImport → SaveSession (YAML on disk)
//	            → LoadSession                → field equality assertion
//
// 这条管道是 v1.4.1 修复的核心路径，但当前 4 个 helper 测试只断言内存中的 helper 输出，
// 没覆盖 YAML 序列化/反序列化是否会丢字段。如果未来有人改 Session 的 yaml tag 或
// 改 SaveSession 的写法，helper 测试可能仍 PASS 但磁盘上读回的 session 字段缺失，
// 用户在 TUI 里又会看到回归症状。本测试在文件层面把这种回归切断。
func TestImportRoundTrip_PreservesFieldsThroughDisk(t *testing.T) {
	imp := importSession{
		Name:     "round-trip-multi-auth",
		Password: "DecryptedSecret",
		SessionData: map[string]interface{}{
			"host":      "10.20.30.40",
			"port":      2222,
			"user":      "ops",
			"auth_type": "password",
			"auth_methods": []securecrt.AuthMethod{
				{Type: "password", Priority: 0, Password: "ENCRYPTED_BLOB_IGNORED_BY_HELPER"},
				{Type: "publickey", Priority: 1, KeyFile: "/home/ops/.ssh/id_ed25519"},
				{Type: "keyboard-interactive", Priority: 2},
				{Type: "gssapi", Priority: 3},
			},
		},
	}

	built := buildXSSHSessionFromImport(imp)

	// 写到磁盘（与生产 import-securecrt 相同的 SaveSession 路径）
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "round-trip-multi-auth.yaml")
	if err := session.SaveSession(built, yamlPath); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// 从磁盘读回
	loaded, err := session.LoadSession(yamlPath)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	// LoadSession 会跑 Validate，AuthType=password 不需要 key_path 检查，应当 Valid
	if !loaded.Valid {
		t.Fatalf("loaded session marked invalid: %v", loaded.Error)
	}

	// 基础字段
	if loaded.Host != "10.20.30.40" {
		t.Errorf("Host: got %q want 10.20.30.40", loaded.Host)
	}
	if loaded.Port != 2222 {
		t.Errorf("Port: got %d want 2222", loaded.Port)
	}
	if loaded.User != "ops" {
		t.Errorf("User: got %q want ops", loaded.User)
	}
	if loaded.AuthType != session.AuthTypePassword {
		t.Errorf("AuthType: got %q want password", loaded.AuthType)
	}

	// 解密后的密码（顶层 + 多 auth 中的 password 项）
	if loaded.Password != "DecryptedSecret" {
		t.Errorf("top-level Password lost through round-trip: got %q", loaded.Password)
	}

	// AuthMethods 完整保留（顺序、类型、KeyPath、解密后密码）
	wantMethods := []session.AuthMethod{
		{Type: "password", Priority: 0, Password: "DecryptedSecret"},
		{Type: "publickey", Priority: 1, KeyPath: "/home/ops/.ssh/id_ed25519"},
		{Type: "keyboard-interactive", Priority: 2},
		{Type: "gssapi", Priority: 3},
	}
	if !reflect.DeepEqual(loaded.AuthMethods, wantMethods) {
		t.Errorf("AuthMethods round-trip mismatch:\n  got:  %#v\n  want: %#v",
			loaded.AuthMethods, wantMethods)
	}
}

// TestImportRoundTrip_PublicKeyAuth 验证 publickey 类型 session 经磁盘 round-trip 后
// 1) 不被误判 [invalid]，2) AuthType 规范化为 key（v1.4.1 修复行为）。
func TestImportRoundTrip_PublicKeyAuth(t *testing.T) {
	imp := importSession{
		Name: "round-trip-publickey",
		SessionData: map[string]interface{}{
			"host":      "10.20.30.41",
			"port":      22,
			"user":      "deploy",
			"auth_type": "publickey",
			"auth_methods": []securecrt.AuthMethod{
				{Type: "publickey", Priority: 0},
				{Type: "password", Priority: 1},
			},
		},
	}

	built := buildXSSHSessionFromImport(imp)

	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "round-trip-publickey.yaml")
	if err := session.SaveSession(built, yamlPath); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	loaded, err := session.LoadSession(yamlPath)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	if !loaded.Valid {
		t.Fatalf("publickey session should be valid after round-trip, err=%v", loaded.Error)
	}
	// Validate 把 publickey 规范化为 key
	if loaded.AuthType != session.AuthTypeKey {
		t.Errorf("AuthType after round-trip: got %q want key (publickey alias)", loaded.AuthType)
	}
	if len(loaded.AuthMethods) != 2 {
		t.Errorf("AuthMethods length: got %d want 2", len(loaded.AuthMethods))
	}
}
