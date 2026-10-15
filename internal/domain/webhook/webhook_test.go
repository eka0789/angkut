package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

func sign(algo Algo, secret string, body []byte) string {
	return computeMAC(algo, secret, body)
}

func TestVerifySignatureAcceptsValidHMAC(t *testing.T) {
	body := []byte(`{"awb":"JNE1","status":"delivered"}`)
	secret := "rahasia-provider"
	sig := sign(AlgoSHA256, secret, body)

	if err := VerifySignature(AlgoSHA256, secret, body, sig); err != nil {
		t.Fatalf("signature sah ditolak: %v", err)
	}
}

func TestVerifySignatureRejectsTamperedBody(t *testing.T) {
	body := []byte(`{"awb":"JNE1","status":"delivered"}`)
	secret := "rahasia-provider"
	sig := sign(AlgoSHA256, secret, body)

	tampered := []byte(`{"awb":"JNE1","status":"cancelled"}`)
	err := VerifySignature(AlgoSHA256, secret, tampered, sig)
	if err == nil {
		t.Fatal("body yang diubah harus ditolak")
	}
	var de *shared.Error
	if !shared.As(err, &de) || de.Code != "WEBHOOK_SIGNATURE_INVALID" {
		t.Errorf("kode error = %v, ingin WEBHOOK_SIGNATURE_INVALID", err)
	}
}

func TestVerifySignatureRejectsWrongSecret(t *testing.T) {
	body := []byte(`{"x":1}`)
	sig := sign(AlgoSHA256, "rahasia-benar", body)
	if err := VerifySignature(AlgoSHA256, "rahasia-salah", body, sig); err == nil {
		t.Fatal("rahasia salah harus ditolak")
	}
}

func TestVerifySignatureHandlesCommonHeaderFormats(t *testing.T) {
	body := []byte(`{"x":1}`)
	secret := "s3cret"
	hexSig := sign(AlgoSHA256, secret, body)

	cases := []string{
		hexSig,
		"sha256=" + hexSig,
		"algo=sha256, sig=" + hexSig,
		strings.ToUpper(hexSig), // sebagian provider mengirim huruf besar
	}
	for _, header := range cases {
		if err := VerifySignature(AlgoSHA256, secret, body, header); err != nil {
			t.Errorf("format header %q ditolak: %v", header, err)
		}
	}
}

func TestVerifySignatureSupportsSHA1(t *testing.T) {
	body := []byte(`{"x":1}`)
	secret := "s"
	sig := sign(AlgoSHA1, secret, body)
	if err := VerifySignature(AlgoSHA1, secret, body, sig); err != nil {
		t.Fatalf("HMAC-SHA1 sah ditolak: %v", err)
	}
	// Signature SHA1 tidak boleh lolos sebagai SHA256.
	if err := VerifySignature(AlgoSHA256, secret, body, sig); err == nil {
		t.Error("signature SHA1 tidak boleh diterima sebagai SHA256")
	}
}

func TestVerifySignatureFailsClosedWithoutSecret(t *testing.T) {
	body := []byte(`{}`)
	if err := VerifySignature(AlgoSHA256, "", body, "apa saja"); err == nil {
		t.Fatal("tanpa rahasia harus GAGAL, bukan lolos")
	}
}

func TestVerifySignatureRequiresHeader(t *testing.T) {
	if err := VerifySignature(AlgoSHA256, "s", []byte(`{}`), ""); err == nil {
		t.Fatal("header kosong harus ditolak")
	}
}

func TestVerifySignatureRejectsUnsupportedAlgo(t *testing.T) {
	if err := VerifySignature(Algo("md5"), "s", []byte(`{}`), "x"); err == nil {
		t.Fatal("algoritma tak didukung harus ditolak")
	}
}

func TestHashPayloadIsStableAndDistinct(t *testing.T) {
	a := HashPayload([]byte(`{"x":1}`))
	b := HashPayload([]byte(`{"x":1}`))
	c := HashPayload([]byte(`{"x":2}`))

	if a != b {
		t.Error("hash payload harus deterministik")
	}
	if a == c {
		t.Error("payload berbeda harus menghasilkan hash berbeda")
	}
	// SHA-256 heksadesimal = 64 karakter.
	if len(a) != 64 {
		t.Errorf("panjang hash = %d, ingin 64", len(a))
	}
}

func TestExtractAWBHandlesProviderVariants(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"kunci awb", map[string]any{"awb": "JNE123"}, "JNE123"},
		{"kunci waybill", map[string]any{"waybill_no": "SC456"}, "SC456"},
		{"kunci resi", map[string]any{"resi": "  JX789  "}, "JX789"},
		{"bersarang di data", map[string]any{"data": map[string]any{"awb": "NESTED1"}}, "NESTED1"},
		{"bersarang di shipment", map[string]any{"shipment": map[string]any{"tracking_number": "NESTED2"}}, "NESTED2"},
		{"tidak ada", map[string]any{"foo": "bar"}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractAWB(tc.payload); got != tc.want {
				t.Errorf("ExtractAWB = %q, ingin %q", got, tc.want)
			}
		})
	}
}

func TestExtractEventTypeAndID(t *testing.T) {
	p := map[string]any{"event_type": "shipment.delivered", "event_id": "evt-1"}
	if got := ExtractEventType(p); got != "shipment.delivered" {
		t.Errorf("event type = %q", got)
	}
	if got := ExtractEventID(p); got != "evt-1" {
		t.Errorf("event id = %q", got)
	}
	// Fallback ke kunci lain.
	if got := ExtractEventType(map[string]any{"status": "in_transit"}); got != "in_transit" {
		t.Errorf("event type fallback = %q", got)
	}
	if got := ExtractEventID(map[string]any{"notification_id": "n-9"}); got != "n-9" {
		t.Errorf("event id fallback = %q", got)
	}
}

func TestEventValidateDefaultsStatus(t *testing.T) {
	e := &Event{Provider: "jne", PayloadHash: "abc"}
	if err := e.Validate(); err != nil {
		t.Fatalf("event valid ditolak: %v", err)
	}
	if e.Status != StatusReceived {
		t.Errorf("status default = %q, ingin received", e.Status)
	}
}

func TestEventValidateRejectsMissingRequired(t *testing.T) {
	if err := (&Event{PayloadHash: "abc"}).Validate(); err == nil {
		t.Error("provider kosong harus ditolak")
	}
	if err := (&Event{Provider: "jne"}).Validate(); err == nil {
		t.Error("payload_hash kosong harus ditolak")
	}
}

// TestSignatureComparisonIsConstantTimeShape memastikan panjang signature
// berbeda tetap ditolak (bukan panic) — hmac.Equal menangani ini.
func TestSignatureComparisonIsConstantTimeShape(t *testing.T) {
	body := []byte(`{"x":1}`)
	if err := VerifySignature(AlgoSHA256, "s", body, "pendek"); err == nil {
		t.Error("signature pendek harus ditolak")
	}
	long := strings.Repeat("a", 200)
	if err := VerifySignature(AlgoSHA256, "s", body, long); err == nil {
		t.Error("signature panjang palsu harus ditolak")
	}
}

// TestReferenceHMAC menegaskan hasil HMAC cocok dengan perhitungan manual
// (mengunci bentuk implementasi terhadap perubahan tak sengaja).
func TestReferenceHMAC(t *testing.T) {
	body := []byte("hello")
	secret := "key"
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	want := hex.EncodeToString(m.Sum(nil))

	if got := computeMAC(AlgoSHA256, secret, body); got != want {
		t.Errorf("computeMAC = %q, ingin %q", got, want)
	}
}
