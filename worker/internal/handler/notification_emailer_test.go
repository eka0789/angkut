package handler_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/angkut/angkut/apps/internal/infrastructure/consumer"
	workerhandler "github.com/angkut/angkut/apps/worker/internal/handler"
)

type fakeSender struct {
	calls []string
	err   error
}

func (f *fakeSender) Send(_ context.Context, to, subject, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, to+"|"+subject)
	return nil
}

func payloadMap(recipients []map[string]any) map[string]any {
	return map[string]any{
		"event_key":  "shipment.delivered",
		"awb":        "JNE260101000001",
		"recipients": recipients,
	}
}

func TestNotificationEmailerMengirimKeSeluruhPenerima(t *testing.T) {
	sender := &fakeSender{}
	h := &workerhandler.NotificationEmailer{Sender: sender}
	err := h.Handle(context.Background(), consumer.Message{
		ID: "evt-1", Payload: payloadMap([]map[string]any{
			{"user_id": "u1", "email": "a@x.test", "title": "Resi sampai", "body": "Paket tiba."},
			{"user_id": "u2", "email": "b@x.test", "title": "Resi sampai", "body": "Paket tiba."},
		}),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(sender.calls) != 2 {
		t.Fatalf("terkirim = %d, ingin 2", len(sender.calls))
	}
	if !strings.Contains(sender.calls[0], "a@x.test") {
		t.Errorf("penerima pertama salah: %v", sender.calls[0])
	}
}

func TestNotificationEmailerTanpaPenerimaBukanKegagalan(t *testing.T) {
	h := &workerhandler.NotificationEmailer{Sender: &fakeSender{}}
	if err := h.Handle(context.Background(), consumer.Message{
		ID: "evt-2", Payload: payloadMap([]map[string]any{}),
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestNotificationEmailerTanpaSenderAdalahGagalPermanen(t *testing.T) {
	h := &workerhandler.NotificationEmailer{}
	err := h.Handle(context.Background(), consumer.Message{
		ID: "evt-3", Payload: payloadMap([]map[string]any{
			{"user_id": "u1", "email": "a@x.test", "title": "Judul", "body": "Isi"},
		}),
	})
	if err == nil || !consumer.IsPermanent(err) {
		t.Fatalf("err = %v, ingin gagal permanen", err)
	}
}

func TestNotificationEmailerPayloadRusakPermanen(t *testing.T) {
	h := &workerhandler.NotificationEmailer{Sender: &fakeSender{}}
	err := h.Handle(context.Background(), consumer.Message{
		ID: "evt-4", Payload: map[string]any{"recipients": "bukan-array"},
	})
	if err == nil || !consumer.IsPermanent(err) {
		t.Fatalf("payload rusak harus gagal permanen; err=%v", err)
	}
}

func TestNotificationEmailerSeluruhGagalDianggapError(t *testing.T) {
	sender := &fakeSender{err: errors.New("smtp down")}
	h := &workerhandler.NotificationEmailer{Sender: sender}
	if err := h.Handle(context.Background(), consumer.Message{
		ID: "evt-5", Payload: payloadMap([]map[string]any{
			{"user_id": "u1", "email": "a@x.test", "title": "Judul", "body": "Isi"},
		}),
	}); err == nil {
		t.Fatal("seluruh email gagal harus menghasilkan error (memicu retry broker)")
	}
}

func TestNotificationEmailerIsPermanentKontrak(t *testing.T) {
	if !consumer.IsPermanent(consumer.Permanent(errors.New("x"))) {
		t.Fatal("kontrak consumer.IsPermanent berubah — perbarui uji ini")
	}
}
