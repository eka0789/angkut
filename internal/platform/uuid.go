// Package platform berisi util murni tanpa dependensi domain/infrastruktur.
package platform

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// NewUUID menghasilkan UUID v7 (berurutan waktu) bila memungkinkan, agar index
// B-tree ramah insert. Fallback ke UUID v4 bila sumber acak gagal.
func NewUUID() string { return newUUIDv7() }

// newUUIDv7 membuat UUID versi 7 sesuai RFC 9562:
// 48 bit unix ms | 4 bit version | 12 bit acak | 2 bit variant | 62 bit acak.
func newUUIDv7() string {
	ms := nowUnixMilli()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Jangan pernah panic karena RNG; pakai v4 sebagai jaring aman.
		return newUUIDv4()
	}
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return format(b)
}

func newUUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return format(b)
}

func format(b [16]byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RandomToken menghasilkan token acak kriptografis dalam base64url (tanpa padding).
func RandomToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = 32
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(out), nil
}

// RandomDigits menghasilkan kode numerik (OTP) dengan panjang n.
func RandomDigits(n int) (string, error) {
	if n <= 0 {
		n = 6
	}
	out := make([]byte, n)
	max := big.NewInt(10)
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = byte('0' + v.Int64())
	}
	return string(out), nil
}

// RandomCodeToken membuat token aman untuk idempotency & nonce internal.
func RandomCodeToken(nBytes int) (string, error) {
	t, err := RandomToken(nBytes)
	if err != nil {
		return "", err
	}
	return "tok_" + t, nil
}
