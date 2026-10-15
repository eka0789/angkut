package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrCiphertextInvalid menandai ciphertext yang rusak atau kunci salah.
var ErrCiphertextInvalid = errors.New("data terenkripsi tidak dapat dibuka")

// Cipher mengenkripsi rahasia at-rest (mis. TOTP secret per user) dengan
// AES-256-GCM. Kunci berasal dari env dan TIDAK PERNAH disimpan di DB — jadi
// dump database saja tidak cukup untuk membuka rahasia user.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher membuat cipher dari kunci base64 (32 byte setelah didekode).
func NewCipher(keyBase64 string) (*Cipher, error) {
	if keyBase64 == "" {
		return nil, errors.New("kunci enkripsi belum diisi (SECRETS_ENCRYPTION_KEY)")
	}
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("kunci enkripsi bukan base64 yang valid: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("kunci enkripsi harus 32 byte (AES-256), dapat %d byte", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt mengembalikan nonce||ciphertext dalam bentuk byte.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	// Nonce di depan ciphertext — dibutuhkan saat dekripsi.
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt membuka hasil Encrypt.
func (c *Cipher) Decrypt(data []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(data) < ns {
		return nil, ErrCiphertextInvalid
	}
	plain, err := c.aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, ErrCiphertextInvalid
	}
	return plain, nil
}

// GenerateEncryptionKey menghasilkan kunci 32-byte baru dalam base64.
// Dipakai operator saat menyiapkan environment (bukan saat runtime request).
func GenerateEncryptionKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
