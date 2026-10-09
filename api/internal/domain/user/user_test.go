package user

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"  Budi@Example.COM ": "budi@example.com",
		"budi@example.com":    "budi@example.com",
		"BUDI@EXAMPLE.COM":    "budi@example.com",
	}
	for in, want := range cases {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, harap %q", in, got, want)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	valid := []string{"Rahasia123", "password1", "a1b2c3d4"}
	for _, p := range valid {
		if err := ValidatePassword(p); err != nil {
			t.Errorf("password %q seharusnya valid: %v", p, err)
		}
	}
	invalid := map[string]string{
		"pendek1":    "kurang dari 8 karakter",
		"tanpaangka": "tidak ada angka",
		"12345678":   "tidak ada huruf",
		"":           "kosong",
	}
	for p, why := range invalid {
		if err := ValidatePassword(p); err == nil {
			t.Errorf("password %q (%s) seharusnya ditolak", p, why)
		}
	}
	if err := ValidatePassword(strings.Repeat("a1", 100)); err == nil {
		t.Error("password terlalu panjang seharusnya ditolak")
	}
}

func TestAccountCanLogin(t *testing.T) {
	now := time.Now()
	verified := now

	base := func() *User {
		return &User{Status: StatusActive, EmailVerifiedAt: &verified}
	}

	if err := base().CanLogin(now); err != nil {
		t.Errorf("akun aktif seharusnya bisa login: %v", err)
	}

	pending := base()
	pending.Status = StatusPending
	if err := pending.CanLogin(now); err == nil {
		t.Error("akun pending seharusnya ditolak")
	}

	suspended := base()
	suspended.Status = StatusSuspended
	if err := suspended.CanLogin(now); err == nil {
		t.Error("akun suspended seharusnya ditolak")
	}

	locked := base()
	until := now.Add(10 * time.Minute)
	locked.LockedUntil = &until
	if err := locked.CanLogin(now); err == nil {
		t.Error("akun terkunci seharusnya ditolak")
	}

	// Kunci yang sudah lewat tidak lagi menghalangi.
	past := now.Add(-time.Minute)
	unlocked := base()
	unlocked.LockedUntil = &past
	if err := unlocked.CanLogin(now); err != nil {
		t.Errorf("kunci kedaluwarsa seharusnya boleh login: %v", err)
	}
}

func TestLockAfterRepeatedFailures(t *testing.T) {
	u := &User{Status: StatusActive}
	now := time.Now()

	for i := 1; i < MaxFailedLogins; i++ {
		if u.RegisterFailure(now) {
			t.Fatalf("terkunci terlalu cepat pada percobaan ke-%d", i)
		}
	}
	if !u.RegisterFailure(now) {
		t.Fatal("seharusnya terkunci setelah mencapai ambang batas")
	}
	if u.LockedUntil == nil {
		t.Fatal("LockedUntil tidak diset")
	}

	// Login berhasil mereset penghitung dan kunci.
	u.RegisterSuccess(now)
	if u.FailedLoginCount != 0 || u.LockedUntil != nil {
		t.Error("RegisterSuccess tidak mereset penghitung/kunci")
	}
	if u.LastLoginAt == nil {
		t.Error("RegisterSuccess tidak mencatat waktu login")
	}
}

func TestSessionLifecycle(t *testing.T) {
	now := time.Now()
	s := NewSession("user-1", "hash", "Chrome", "UA", "127.0.0.1", time.Hour)

	if !s.IsActive(now) {
		t.Error("sesi baru seharusnya aktif")
	}
	s.Revoke(now)
	if s.IsActive(now) {
		t.Error("sesi yang dicabut seharusnya tidak aktif")
	}
	// Mencabut dua kali tidak mengubah waktu pencabutan pertama.
	first := *s.RevokedAt
	s.Revoke(now.Add(time.Minute))
	if !s.RevokedAt.Equal(first) {
		t.Error("Revoke kedua seharusnya tidak mengubah revoked_at")
	}

	expired := NewSession("user-1", "hash2", "", "", "", time.Second)
	if expired.IsActive(now.Add(2 * time.Second)) {
		t.Error("sesi kedaluwarsa seharusnya tidak aktif")
	}
}

func TestDeviceLabel(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Windows NT 10.0) Chrome/120.0":          "Chrome di Windows",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) Safari/604": "Safari di iOS",
		"Mozilla/5.0 (Linux; Android 14) Edg/120":             "Edge di Android",
		"curl/8.0": "curl",
	}
	for ua, want := range cases {
		if got := SummarizeUserAgent(ua); got != want {
			t.Errorf("SummarizeUserAgent(%q) = %q, harap %q", ua, got, want)
		}
	}

	// Label eksplisit menang atas user-agent.
	s := NewSession("u", "h", "Laptop Kerja", "curl/8.0", "", time.Hour)
	if s.DeviceLabel() != "Laptop Kerja" {
		t.Errorf("device label = %q", s.DeviceLabel())
	}

	// Tidak pernah kosong.
	empty := NewSession("u", "h", "", "", "", time.Hour)
	if empty.DeviceLabel() == "" {
		t.Error("label perangkat tidak boleh kosong")
	}
}

func TestRecoveryCodes(t *testing.T) {
	plain, codes, err := GenerateRecoveryCodes("user-1")
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(plain) != RecoveryCodeCount || len(codes) != RecoveryCodeCount {
		t.Fatalf("jumlah kode = %d/%d, harap %d", len(plain), len(codes), RecoveryCodeCount)
	}

	// Kode mentah tidak boleh tersimpan apa adanya.
	for i, p := range plain {
		if codes[i].CodeHash == p {
			t.Error("kode pemulihan tersimpan tanpa hash")
		}
		if !MatchRecoveryCode(p, codes[i].CodeHash) {
			t.Errorf("kode %q tidak cocok dengan hash-nya", p)
		}
	}

	// Normalisasi: huruf besar dan tanda hubung boleh berbeda saat dimasukkan.
	if !MatchRecoveryCode(strings.ToUpper(plain[0]), codes[0].CodeHash) {
		t.Error("kode pemulihan harus menerima huruf besar")
	}
	if !MatchRecoveryCode(strings.ReplaceAll(plain[0], "-", ""), codes[0].CodeHash) {
		t.Error("kode pemulihan harus menerima tanpa tanda hubung")
	}
	if MatchRecoveryCode("salah-salah", codes[0].CodeHash) {
		t.Error("kode salah seharusnya tidak cocok")
	}
}

func TestOpaqueTokens(t *testing.T) {
	raw1, hash1, err := GenerateOpaqueToken()
	if err != nil {
		t.Fatalf("GenerateOpaqueToken: %v", err)
	}
	raw2, hash2, err := GenerateOpaqueToken()
	if err != nil {
		t.Fatalf("GenerateOpaqueToken: %v", err)
	}
	if raw1 == raw2 {
		t.Error("token harus unik antar pemanggilan")
	}
	if hash1 == hash2 {
		t.Error("hash token harus unik")
	}
	if HashToken(raw1) != hash1 {
		t.Error("HashToken tidak deterministik")
	}
	if len(hash1) != 64 {
		t.Errorf("hash SHA-256 harus 64 karakter hex, dapat %d", len(hash1))
	}
}

func TestMarkEmailVerifiedActivatesPending(t *testing.T) {
	u := NewUser("budi@example.com", "Budi")
	if u.Status != StatusPending {
		t.Fatalf("pengguna baru harus berstatus pending, dapat %s", u.Status)
	}
	u.MarkEmailVerified(time.Now())
	if !u.IsEmailVerified() || u.Status != StatusActive {
		t.Errorf("setelah verifikasi: status=%s verified=%v", u.Status, u.IsEmailVerified())
	}
}
