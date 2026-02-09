package securecrt

import (
	"fmt"
	"testing"
)

func TestDecryptPasswordV2Real(t *testing.T) {
	passphrase := "Cncsi12345"
	encrypted := "03:114144e49c3127e10915dd3995c81eec61ed87728b998d75fefc285b9b2f998c56c8151b372f9dfbfc6120817faf5b9411f9abf3e67f9349bf7d47eb5b53fb8002afdd115ffd82c6ebc7097995745ac2"

	result, err := decryptPasswordV2(encrypted, passphrase)
	if err != nil {
		t.Fatalf("decryptPasswordV2 failed: %v", err)
	}

	fmt.Printf("Decrypted password: %q (len=%d)\n", result, len(result))

	if result == "" {
		t.Fatal("decrypted password is empty")
	}
}

func TestLoadSessions(t *testing.T) {
	config := Config{
		SessionPath: "/Users/david/.xsc/securecrt_sessions",
		Password:    "Cncsi12345",
	}

	sessions, err := LoadSessions(config)
	if err != nil {
		// SecureCRT 路径不存在时跳过测试
		t.Skipf("SecureCRT session path does not exist, skipping: %v", err)
	}

	pwCount := 0
	for _, s := range sessions {
		auth := "agent"
		if s.EncryptedPassword != "" {
			auth = "password"
			pwCount++
		}
		fmt.Printf("%-40s host=%-16s user=%-8s auth=%s\n",
			s.Name, s.Hostname, s.Username, auth)
	}
	fmt.Printf("\nTotal: %d sessions, %d with password\n", len(sessions), pwCount)

	if pwCount == 0 {
		t.Log("Warning: no sessions with encrypted password found")
	}
}
