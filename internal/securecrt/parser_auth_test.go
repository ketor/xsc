package securecrt

import (
	"testing"
)

func TestParseAuthMethods(t *testing.T) {
	tests := []struct {
		name       string
		authString string
		expected   []string // 期望的认证类型列表
	}{
		{
			name:       "single password",
			authString: "password",
			expected:   []string{"password"},
		},
		{
			name:       "password and publickey",
			authString: "password,publickey",
			expected:   []string{"password", "publickey"},
		},
		{
			name:       "multiple methods",
			authString: "publickey,password,keyboard-interactive",
			expected:   []string{"publickey", "password", "keyboard-interactive"},
		},
		{
			name:       "with spaces",
			authString: "publickey, password, keyboard-interactive",
			expected:   []string{"publickey", "password", "keyboard-interactive"},
		},
		{
			name:       "case insensitive",
			authString: "PASSWORD,PublicKey,GSSAPI",
			expected:   []string{"password", "publickey", "gssapi"},
		},
		{
			name:       "empty string",
			authString: "",
			expected:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &Session{}
			parseAuthMethods(session, tt.authString)

			if len(session.AuthMethods) != len(tt.expected) {
				t.Errorf("expected %d auth methods, got %d", len(tt.expected), len(session.AuthMethods))
				return
			}

			for i, expected := range tt.expected {
				if session.AuthMethods[i].Type != expected {
					t.Errorf("auth method %d: expected type %s, got %s", i, expected, session.AuthMethods[i].Type)
				}
				if session.AuthMethods[i].Priority != i {
					t.Errorf("auth method %d: expected priority %d, got %d", i, i, session.AuthMethods[i].Priority)
				}
			}
		})
	}
}

func TestSessionConvertToXSCSession(t *testing.T) {
	session := &Session{
		Name:     "test-session",
		Hostname: "192.168.1.100",
		Port:     22,
		Username: "root",
		AuthMethods: []AuthMethod{
			{Type: "publickey", Priority: 0, KeyFile: "/path/to/key"},
			{Type: "password", Priority: 1},
		},
		UseAgent:      true,
		PublicKeyFile: "/path/to/key",
	}

	result := session.ConvertToXSCSession()

	// 检查基本字段
	if result["host"] != "192.168.1.100" {
		t.Errorf("expected host 192.168.1.100, got %v", result["host"])
	}
	if result["user"] != "root" {
		t.Errorf("expected user root, got %v", result["user"])
	}

	// 检查 auth_methods 是否存在
	authMethods, ok := result["auth_methods"].([]AuthMethod)
	if !ok {
		t.Error("expected auth_methods to be []AuthMethod")
		return
	}

	if len(authMethods) != 2 {
		t.Errorf("expected 2 auth methods, got %d", len(authMethods))
	}

	// 检查向后兼容的 auth_type
	if result["auth_type"] != "publickey" {
		t.Errorf("expected auth_type publickey, got %v", result["auth_type"])
	}
}

func TestSessionConvertToXSCSessionWithPassword(t *testing.T) {
	session := &Session{
		Name:              "test-session",
		Hostname:          "192.168.1.100",
		Port:              22,
		Username:          "root",
		EncryptedPassword: "03:encrypted_data_here",
		AuthMethods: []AuthMethod{
			{Type: "password", Priority: 0, Password: "03:encrypted_data_here"},
		},
	}

	result := session.ConvertToXSCSession()

	authMethods, ok := result["auth_methods"].([]AuthMethod)
	if !ok {
		t.Error("expected auth_methods to be []AuthMethod")
		return
	}

	if len(authMethods) != 1 {
		t.Errorf("expected 1 auth method, got %d", len(authMethods))
		return
	}

	if authMethods[0].Password != "03:encrypted_data_here" {
		t.Error("expected encrypted password to be preserved in auth method")
	}
}

func TestSessionConvertToXSCSessionDefaultAuth(t *testing.T) {
	// 测试当没有指定认证方式时的默认行为
	session := &Session{
		Name:     "test-session",
		Hostname: "192.168.1.100",
		Port:     22,
		Username: "root",
		// 没有 AuthMethods，也没有密码或密钥
	}

	result := session.ConvertToXSCSession()

	authMethods, ok := result["auth_methods"].([]AuthMethod)
	if !ok {
		t.Error("expected auth_methods to be []AuthMethod")
		return
	}

	if len(authMethods) != 1 {
		t.Errorf("expected 1 default auth method, got %d", len(authMethods))
		return
	}

	if authMethods[0].Type != "agent" {
		t.Errorf("expected default auth type to be agent, got %s", authMethods[0].Type)
	}
}
