// Package securecrt provides functionality to parse and decrypt SecureCRT session files
package securecrt

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AuthMethod represents an authentication method with its configuration
type AuthMethod struct {
	Type     string // "password", "publickey", "keyboard-interactive", "agent"
	Password string // 解密后的密码（延迟解密时为空）
	KeyFile  string // 公钥文件路径
	Priority int    // 优先级顺序
}

// Session represents a SecureCRT session
type Session struct {
	Name               string
	Hostname           string
	Port               int
	Username           string
	Password           string // 解密后的密码（延迟解密时为空）
	EncryptedPassword  string // 原始加密密码（用于延迟解密）
	Protocol           string
	Emulation          string
	FilePath           string
	Folder             string
	AuthMethods        []AuthMethod // 认证方法列表，按优先级排序
	UseAgent           bool         // 是否使用 SSH Agent
	PublicKeyFile      string       // 公钥文件路径
	UseGlobalPublicKey bool         // 是否使用全局公钥设置
	IdentityFilename   string       // Identity 文件名（S:"Identity Filename V2"）
}

// Config represents SecureCRT configuration
type Config struct {
	SessionPath string
	Password    string // Master password for decryption
}

// LoadSessions loads all SecureCRT sessions from the given path
func LoadSessions(config Config) ([]*Session, error) {
	if _, err := os.Stat(config.SessionPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SecureCRT session path does not exist: %s", config.SessionPath)
	}

	var sessions []*Session

	err := filepath.Walk(config.SessionPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// SecureCRT sessions are .ini files
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".ini") {
			// Skip __FolderData__.ini files
			if info.Name() == "__FolderData__.ini" || info.Name() == "Default.ini" {
				return nil
			}
			session, err := parseSessionFile(path, config.SessionPath, config.Password)
			if err != nil {
				return nil
			}
			// 只添加有 hostname 的 session
			if session.Hostname != "" {
				sessions = append(sessions, session)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// parseSessionFile parses a single SecureCRT session file
func parseSessionFile(filePath, basePath, masterPassword string) (*Session, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	session := &Session{
		FilePath: filePath,
		Port:     22,
		Protocol: "SSH2",
	}

	// Get folder path relative to base path
	relPath, _ := filepath.Rel(basePath, filepath.Dir(filePath))
	if relPath != "." {
		session.Folder = relPath
	}

	// Get session name from filename (without .ini extension)
	baseName := filepath.Base(filePath)
	session.Name = strings.TrimSuffix(baseName, ".ini")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		// Parse key=value pairs (SecureCRT uses format like S:"Hostname"=value)
		if idx := strings.Index(line, "="); idx > 0 {
			rawKey := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])

			// 保留原始 key 用于特殊匹配
			key := cleanKey(rawKey)

			switch key {
			case "hostname":
				session.Hostname = value
			case "[ssh2] port":
				// SecureCRT 使用 D:"[SSH2] Port"=00000016 格式（十六进制）
				if port, err := strconv.ParseInt(value, 16, 32); err == nil && port > 0 {
					session.Port = int(port)
				}
			case "username":
				session.Username = value
			case "password v2":
				// V2 密码格式: "02:hex..." 或 "03:hex..."
				// 只保存加密密码，延迟到连接时解密
				if value != "" {
					session.EncryptedPassword = value
				}
			case "password":
				// V1 密码格式（旧版本 SecureCRT < 7.3.3）
				if session.EncryptedPassword == "" && value != "" {
					session.EncryptedPassword = value
				}
			case "authentication":
				// 认证方式列表，用逗号分隔，如 "password,publickey,keyboard-interactive"
				if value != "" {
					parseAuthMethods(session, value)
				}
			case "ssh2 authentications v2", "ssh2 authentications", "authentications":
				// SecureCRT 使用 S:"SSH2 Authentications V2" 字段，cleanKey 后变成 "ssh2 authentications v2"
				if value != "" {
					parseAuthMethods(session, value)
				}
			case "public key file":
				session.PublicKeyFile = value
			case "identity filename v2":
				// SecureCRT 使用 S:"Identity Filename V2" 字段指定私钥文件
				session.IdentityFilename = value
			case "use global public key":
				// SecureCRT 使用 D:"Use Global Public Key"=00000001 表示使用全局公钥
				if value == "1" || strings.ToLower(value) == "true" {
					session.UseGlobalPublicKey = true
				}
			case "use agent":
				// SecureCRT 使用 B: 类型表示布尔值，0 或 1
				if value == "1" || strings.ToLower(value) == "true" {
					session.UseAgent = true
				}
			case "protocol name":
				session.Protocol = value
			case "emulation":
				session.Emulation = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return session, nil
}

// cleanKey removes type prefix and quotes from SecureCRT key names
// e.g., S:"Hostname" -> hostname, D:"[SSH2] Port" -> [ssh2] port
func cleanKey(key string) string {
	key = strings.TrimSpace(key)

	// Remove type prefix (S:, D:, B:, etc.)
	if idx := strings.Index(key, ":"); idx > 0 && idx < 3 {
		key = key[idx+1:]
	}

	// Remove quotes
	key = strings.Trim(key, "\"")

	return strings.ToLower(key)
}

// parseAuthMethods parses the authentication string and populates AuthMethods list
// SecureCRT format: "password,publickey,keyboard-interactive"
func parseAuthMethods(session *Session, authString string) {
	// Split by comma and trim spaces
	methods := strings.Split(authString, ",")
	for i, method := range methods {
		method = strings.TrimSpace(strings.ToLower(method))
		if method == "" {
			continue
		}

		auth := AuthMethod{
			Type:     method,
			Priority: i,
		}

		// Map SecureCRT method names to internal names
		switch method {
		case "password":
			// 密码将在延迟解密时填充
			auth.Type = "password"
		case "publickey", "rsa", "dsa", "ecdsa", "ed25519":
			auth.Type = "publickey"
		case "keyboard-interactive":
			auth.Type = "keyboard-interactive"
		case "gssapi", "gssapi-keyex", "gssapi-with-mic":
			auth.Type = "gssapi"
		case "none":
			auth.Type = "none"
		}

		session.AuthMethods = append(session.AuthMethods, auth)
	}
}

// decryptPasswordV2 decrypts a SecureCRT V2 encrypted password
// Format: "prefix:hex_data" where prefix is "02" or "03"
func decryptPasswordV2(encryptedStr, passphrase string) (string, error) {
	// 分离 prefix 和密文
	parts := strings.SplitN(encryptedStr, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid password v2 format")
	}

	prefix := parts[0]
	hexData := parts[1]

	ciphertext, err := hex.DecodeString(hexData)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex: %w", err)
	}

	var plaintext []byte

	switch prefix {
	case "02":
		plaintext, err = decryptV2Prefix02(ciphertext, passphrase)
	case "03":
		plaintext, err = decryptV2Prefix03(ciphertext, passphrase)
	default:
		return "", fmt.Errorf("unknown password v2 prefix: %s", prefix)
	}

	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// decryptV2Prefix02 使用 SHA256(passphrase) 作为 AES-256-CBC 密钥，IV 全零
func decryptV2Prefix02(ciphertext []byte, passphrase string) ([]byte, error) {
	key := sha256.Sum256([]byte(passphrase))
	iv := make([]byte, aes.BlockSize)

	return decryptAESCBC(ciphertext, key[:], iv)
}

// decryptV2Prefix03 使用 bcrypt_pbkdf2 派生密钥
// ciphertext 前 16 字节是 salt，剩余是 AES-256-CBC 加密数据
func decryptV2Prefix03(ciphertext []byte, passphrase string) ([]byte, error) {
	if len(ciphertext) < 16 {
		return nil, fmt.Errorf("ciphertext too short")
	}

	salt := ciphertext[:16]
	encrypted := ciphertext[16:]

	// bcrypt_pbkdf2 派生 32 字节密钥 + 16 字节 IV = 48 字节
	kdfBytes, err := bcryptPbkdfKey([]byte(passphrase), salt, 16, 32+aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("bcrypt_pbkdf2 failed: %w", err)
	}

	aesKey := kdfBytes[:32]
	iv := kdfBytes[32 : 32+aes.BlockSize]

	return decryptAESCBC(encrypted, aesKey, iv)
}

// decryptAESCBC 解密 AES-CBC 数据并验证 LVC 格式
// 解密后格式: [4字节小端长度][明文][32字节SHA256校验][填充]
func decryptAESCBC(ciphertext, key, iv []byte) ([]byte, error) {
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length not aligned to block size")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	decryptor := cipher.NewCBCDecrypter(block, iv)
	padded := make([]byte, len(ciphertext))
	decryptor.CryptBlocks(padded, ciphertext)

	// 解析 LVC 格式: length(4) + value(length) + checksum(32) + padding
	if len(padded) < 4 {
		return nil, fmt.Errorf("decrypted data too short")
	}

	plaintextLen := binary.LittleEndian.Uint32(padded[:4])
	if int(plaintextLen) > len(padded)-4 {
		return nil, fmt.Errorf("invalid plaintext length")
	}

	plaintext := padded[4 : 4+plaintextLen]

	// 验证 SHA256 校验和
	if int(4+plaintextLen+sha256.Size) > len(padded) {
		return nil, fmt.Errorf("missing checksum")
	}

	checksum := padded[4+plaintextLen : 4+plaintextLen+sha256.Size]
	expected := sha256.Sum256(plaintext)
	for i := 0; i < sha256.Size; i++ {
		if checksum[i] != expected[i] {
			return nil, fmt.Errorf("checksum mismatch: wrong passphrase?")
		}
	}

	return plaintext, nil
}

// decryptPasswordV1 decrypts a SecureCRT V1 encrypted password (pre-7.3.3)
// 使用 Blowfish-CBC，硬编码密钥
func decryptPasswordV1(encryptedHex string) (string, error) {
	// V1 密码以 'u' 开头
	if len(encryptedHex) > 0 && encryptedHex[0] == 'u' {
		encryptedHex = encryptedHex[1:]
	}

	ciphertext, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex: %w", err)
	}

	if len(ciphertext) <= 8 {
		return "", fmt.Errorf("ciphertext too short")
	}

	// V1 使用 Blowfish-CBC，硬编码两组密钥
	// 这里简化处理，V1 格式在现代 SecureCRT 中很少使用
	return "", fmt.Errorf("V1 password decryption not supported, please upgrade SecureCRT session files")
}

// DecryptPassword 解密密码（延迟解密）
func DecryptPassword(encryptedPassword, masterPassword string) (string, error) {
	if encryptedPassword == "" || masterPassword == "" {
		return "", fmt.Errorf("no encrypted password or master password")
	}

	// 判断是 V2 还是 V1 格式
	if strings.Contains(encryptedPassword, ":") {
		return decryptPasswordV2(encryptedPassword, masterPassword)
	}
	return decryptPasswordV1(encryptedPassword)
}

// HasEncryptedPassword 检查是否有加密密码
func (s *Session) HasEncryptedPassword() bool {
	return s.EncryptedPassword != ""
}

// ConvertToXSCSession converts a SecureCRT session to xsc session format
func (s *Session) ConvertToXSCSession() map[string]interface{} {
	result := map[string]interface{}{
		"host": s.Hostname,
		"port": s.Port,
		"user": s.Username,
	}

	// 如果没有解析到认证方式列表，使用默认的检测逻辑
	if len(s.AuthMethods) == 0 {
		// 构建默认的认证方式列表（按优先级）
		if s.PublicKeyFile != "" {
			s.AuthMethods = append(s.AuthMethods, AuthMethod{
				Type:     "publickey",
				KeyFile:  s.PublicKeyFile,
				Priority: 0,
			})
		}
		if s.UseAgent {
			s.AuthMethods = append(s.AuthMethods, AuthMethod{
				Type:     "agent",
				Priority: len(s.AuthMethods),
			})
		}
		if s.EncryptedPassword != "" || s.Password != "" {
			s.AuthMethods = append(s.AuthMethods, AuthMethod{
				Type:     "password",
				Priority: len(s.AuthMethods),
			})
		}
		// 如果都没有，添加 agent 作为默认值
		if len(s.AuthMethods) == 0 {
			s.AuthMethods = append(s.AuthMethods, AuthMethod{
				Type:     "agent",
				Priority: 0,
			})
		}
	}

	// 填充密码到对应的认证方法（如果有加密密码）
	if s.EncryptedPassword != "" {
		for i := range s.AuthMethods {
			if s.AuthMethods[i].Type == "password" {
				s.AuthMethods[i].Password = s.EncryptedPassword
				break
			}
		}
	}

	// 填充公钥文件路径到对应的认证方法
	if s.PublicKeyFile != "" {
		for i := range s.AuthMethods {
			if s.AuthMethods[i].Type == "publickey" && s.AuthMethods[i].KeyFile == "" {
				s.AuthMethods[i].KeyFile = s.PublicKeyFile
				break
			}
		}
	}

	// 如果指定了 Identity Filename V2，使用它作为公钥文件
	if s.IdentityFilename != "" {
		for i := range s.AuthMethods {
			if s.AuthMethods[i].Type == "publickey" && s.AuthMethods[i].KeyFile == "" {
				s.AuthMethods[i].KeyFile = s.IdentityFilename
				break
			}
		}
	}

	// 标记使用全局公钥的认证方法
	if s.UseGlobalPublicKey {
		for i := range s.AuthMethods {
			if s.AuthMethods[i].Type == "publickey" && s.AuthMethods[i].KeyFile == "" {
				// 标记使用全局公钥，后续会自动查找 ~/.ssh/ 下的默认密钥
				s.AuthMethods[i].KeyFile = ""
			}
		}
	}

	// 传递认证方法列表
	result["auth_methods"] = s.AuthMethods

	// 为了向后兼容，仍然提供首选的 auth_type
	if len(s.AuthMethods) > 0 {
		result["auth_type"] = s.AuthMethods[0].Type
		if s.AuthMethods[0].Type == "password" {
			if s.Password != "" {
				result["password"] = s.Password
			} else if s.EncryptedPassword != "" {
				result["encrypted_password"] = s.EncryptedPassword
			}
		} else if s.AuthMethods[0].Type == "publickey" {
			result["key_path"] = s.PublicKeyFile
		}
	}

	return result
}
