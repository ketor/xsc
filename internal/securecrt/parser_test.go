package securecrt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestDecryptPasswordV2Synthetic(t *testing.T) {
	const (
		passphrase = "synthetic-test-passphrase"
		plaintext  = "synthetic-test-password"
	)

	payload := make([]byte, 4+len(plaintext)+sha256.Size)
	binary.LittleEndian.PutUint32(payload[:4], uint32(len(plaintext)))
	copy(payload[4:], plaintext)
	checksum := sha256.Sum256([]byte(plaintext))
	copy(payload[4+len(plaintext):], checksum[:])

	padding := aes.BlockSize - len(payload)%aes.BlockSize
	payload = append(payload, make([]byte, padding)...)

	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}
	ciphertext := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(ciphertext, payload)

	got, err := decryptPasswordV2("02:"+hex.EncodeToString(ciphertext), passphrase)
	if err != nil {
		t.Fatalf("decryptPasswordV2: %v", err)
	}
	if got != plaintext {
		t.Fatalf("decrypted password mismatch")
	}
}
