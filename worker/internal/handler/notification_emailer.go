package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/angkut/angkut/apps/internal/infrastructure/consumer"
)

// EmailSender adalah port pengiriman email (diimplementasi adapter kanal —
// dev: tulis ke log; produksi: SMTP/provider). Dipisah agar handler bisa
// diuji tanpa jaringan.
type EmailSender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// NotificationEmailer mengirim email notifikasi dari event outbox
// `notification.send`. Isi pesan SUDAH dirender di sisi API (template kustom
// organisasi + preferensi sudah diterapkan saat fan-out), jadi handler ini
// murni pengantar: validasi bentuk, kirim per penerima, laporkan hasil.
//
// Idempotensi: dedup event_id oleh kerangka consumer — pesan yang datang dua
// kali (at-least-once) tidak mengirim ulang email.
type NotificationEmailer struct {
	Sender EmailSender
	Log    *slog.Logger
}

// emailItem adalah satu penerima dalam payload notification.send.
type emailItem struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// Handle memproses satu pesan notification.send.
func (h *NotificationEmailer) Handle(ctx context.Context, msg consumer.Message) error {
	log := h.Log
	if log == nil {
		log = slog.Default()
	}
	if msg.Payload == nil {
		return consumer.Permanent(fmt.Errorf("payload notification.send kosong"))
	}
	raw, err := json.Marshal(msg.Payload)
	if err != nil {
		return consumer.Permanent(fmt.Errorf("payload tidak bisa diserialisasi: %w", err))
	}
	var payload struct {
		EventKey   string      `json:"event_key"`
		AWB        string      `json:"awb"`
		Recipients []emailItem `json:"recipients"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return consumer.Permanent(fmt.Errorf("payload notification.send rusak: %w", err))
	}
	if len(payload.Recipients) == 0 {
		return nil // tidak ada opt-in email — bukan kegagalan
	}
	if h.Sender == nil {
		// Tanpa adapter email, pekerjaan ini tidak akan pernah berhasil —
		// jangan buang retry untuk hal yang permanen.
		return consumer.Permanent(fmt.Errorf("adapter email belum dikonfigurasi"))
	}

	sent, failed := 0, 0
	for _, rc := range payload.Recipients {
		if rc.Email == "" || rc.Title == "" {
			failed++
			continue
		}
		if err := h.Sender.Send(ctx, rc.Email, rc.Title, rc.Body); err != nil {
			failed++
			log.Warn("email notifikasi gagal terkirim",
				"to", rc.Email, "event_key", payload.EventKey, "awb", payload.AWB, "error", err)
			continue
		}
		sent++
	}
	log.Info("email notifikasi diproses",
		"event_key", payload.EventKey, "awb", payload.AWB,
		"terkirim", sent, "gagal", failed)
	if sent == 0 && failed > 0 {
		return fmt.Errorf("seluruh email gagal terkirim (%d penerima)", failed)
	}
	return nil
}
