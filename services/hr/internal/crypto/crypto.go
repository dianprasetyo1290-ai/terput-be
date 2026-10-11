package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
)

type Service struct {
	encKey   []byte
	blindKey []byte
}

func New(encKey []byte, blindKey string) (*Service, error) {
	if len(encKey) != 32 {
		return nil, errors.New("crypto: encryption key must be 32 bytes for AES-256")
	}
	if len(blindKey) < 16 {
		return nil, errors.New("crypto: blind index key must be at least 16 bytes")
	}
	return &Service{
		encKey:   encKey,
		blindKey: []byte(blindKey),
	}, nil
}

// Encrypt encrypts plaintext with AES-256-GCM using aad (Additional Authenticated Data).
// Returns binary (nonce || ciphertext || tag).
func (s *Service) Encrypt(plaintext, aad string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: GCM init: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce gen: %w", err)
	}

	return gcm.Seal(nonce, nonce, []byte(plaintext), []byte(aad)), nil
}

// Decrypt reverses Encrypt. aad must match what was used during encryption.
func (s *Service) Decrypt(raw []byte, aad string) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", fmt.Errorf("crypto: cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: GCM init: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("crypto: ciphertext too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return "", fmt.Errorf("crypto: decryption failed: %w", err)
	}
	return string(plaintext), nil
}

// BlindIndex returns a stable HMAC-SHA256 hex digest of the value.
func (s *Service) BlindIndex(value, aad string) string {
	if value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.blindKey)
	mac.Write([]byte(aad))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// EncryptWithIndex encrypts plaintext and produces blind index.
func (s *Service) EncryptWithIndex(plaintext, aad string) ([]byte, string, error) {
	if plaintext == "" {
		return nil, "", nil
	}
	c, err := s.Encrypt(plaintext, aad)
	if err != nil {
		return nil, "", err
	}
	b := s.BlindIndex(plaintext, aad)
	return c, b, nil
}

// EncryptAmount encrypts an int64 amount as string.
func (s *Service) EncryptAmount(amount int64, aad string) ([]byte, error) {
	return s.Encrypt(strconv.FormatInt(amount, 10), aad)
}

// DecryptAmount decrypts into int64.
func (s *Service) DecryptAmount(raw []byte, aad string) (int64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	str, err := s.Decrypt(raw, aad)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(str, 10, 64)
}
