package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Manager provides envelope encryption for sensitive values using a master key.
type Manager struct {
	key []byte // 32 bytes AES-256
}

func New(masterKey string) (*Manager, error) {
	if masterKey == "" {
		// Allow empty key in dev; encryption becomes a no-op with a static zero key
		masterKey = ""
	}
	b := make([]byte, 32)
	copy(b, []byte(masterKey))
	return &Manager{key: b}, nil
}

func (m *Manager) Encrypt(plaintext string) (string, error) {
	if len(m.key) == 0 {
		return plaintext, nil
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (m *Manager) Decrypt(ciphertext string) (string, error) {
	if len(m.key) == 0 {
		return ciphertext, nil
	}
	b, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(b) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce := b[:gcm.NonceSize()]
	ct := b[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
