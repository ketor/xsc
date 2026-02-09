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
			name: "password auth without password",
			session: Session{
				Host:     "192.168.1.1",
				Port:     22,
				User:     "root",
				AuthType: AuthTypePassword,
			},
			wantErr: true,
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

func TestLoadAndSaveSession(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "session-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建测试会话
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

	// 保存会话
	sessionPath := filepath.Join(tmpDir, "test-session.yaml")
	err = SaveSession(session, sessionPath)
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Error("Session file should exist")
	}

	// 加载会话
	loadedSession, err := LoadSession(sessionPath)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}

	// 验证字段
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

func TestLoadSessionInvalidYAML(t *testing.T) {
	// 创建临时文件包含无效 YAML
	tmpDir, err := os.MkdirTemp("", "session-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	invalidYAML := []byte("invalid: yaml: content: [")
	sessionPath := filepath.Join(tmpDir, "invalid.yaml")
	err = os.WriteFile(sessionPath, invalidYAML, 0600)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// 加载应该返回会话但标记为无效
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

func TestAuthMethodTypes(t *testing.T) {
	// 测试认证方法
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
