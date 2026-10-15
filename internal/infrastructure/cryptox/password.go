// Package cryptox menyediakan primitif kriptografi aplikasi: hashing password
// (Argon2id) dan enkripsi simetris (AES-GCM) untuk rahasia at-rest.
//
// Prinsip: parameter TIDAK di-hardcode di sini — dibaca dari config, dan selalu
// ikut disimpan/di-encode bersama hash sehingga verifikasi tetap bisa dilakukan
// walau parameter dinaikkan di kemudian hari.
package cryptox

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params adalah parameter biaya Argon2id.
type Argon2Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params dipakai bila config tidak memberi nilai. Mengikuti
// rekomendasi OWASP untuk Argon2id (64 MiB, 3 iterasi, paralelisme 2).
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		MemoryKiB:   64 * 1024,
		Iterations:  3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// ErrInvalidHash menandai format hash yang tidak dikenali.
var ErrInvalidHash = errors.New("format hash password tidak dikenali")

// ErrIncompatibleVersion menandai hash dari versi Argon2 yang berbeda.
var ErrIncompatibleVersion = errors.New("versi argon2 tidak kompatibel")

// HashPassword menghasilkan hash Argon2id dalam format PHC (satu string berisi
// parameter + salt + hash), sehingga parameter dapat berubah tanpa migrasi kolom.
func HashPassword(password string, p Argon2Params) (string, error) {
	if p.MemoryKiB == 0 {
		p = DefaultArgon2Params()
	}
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.KeyLength)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword membandingkan password dengan hash PHC secara constant-time.
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	other := argon2.IDKey([]byte(password), salt, p.Iterations, p.MemoryKiB, p.Parallelism, uint32(len(key)))
	return subtle.ConstantTimeCompare(key, other) == 1, nil
}

// NeedsRehash melaporkan apakah hash lama memakai parameter lebih lemah dari
// yang berlaku sekarang — dipakai untuk menaikkan biaya hash saat user login.
func NeedsRehash(encoded string, want Argon2Params) bool {
	p, _, _, err := decodeHash(encoded)
	if err != nil {
		return true
	}
	return p.MemoryKiB < want.MemoryKiB || p.Iterations < want.Iterations || p.Parallelism < want.Parallelism
}

func decodeHash(encoded string) (p Argon2Params, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return p, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return p, nil, nil, ErrIncompatibleVersion
	}

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.MemoryKiB, &p.Iterations, &p.Parallelism); err != nil {
		return p, nil, nil, ErrInvalidHash
	}

	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return p, nil, nil, ErrInvalidHash
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return p, nil, nil, ErrInvalidHash
	}
	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(key))
	return p, salt, key, nil
}
