// Package observability menyiapkan logging terstruktur (slog JSON) dan metrik.
// Logger menyisipkan korelasi request/trace/user/org ke setiap baris log.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// ctxKey menjaga kunci context tetap privat.
type ctxKey struct{ name string }

var (
	keyRequestID = ctxKey{"request_id"}
	keyTraceID   = ctxKey{"trace_id"}
	keyUserID    = ctxKey{"user_id"}
	keyOrgID     = ctxKey{"organization_id"}
	keyModule    = ctxKey{"module"}
	keyActorType = ctxKey{"actor_type"}
)

// New membuat logger JSON ke stdout dengan level dari config.
func New(level string, env string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lv}
	h := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(h).With(slog.String("service", "angkut"), slog.String("env", env))
	return logger
}

// WithRequestID menyisipkan request_id ke context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

func RequestID(ctx context.Context) string { return str(ctx, keyRequestID) }

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTraceID, id)
}

func TraceID(ctx context.Context) string { return str(ctx, keyTraceID) }

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyUserID, id)
}

func UserID(ctx context.Context) string { return str(ctx, keyUserID) }

func WithOrganizationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyOrgID, id)
}

func OrganizationID(ctx context.Context) string { return str(ctx, keyOrgID) }

func WithActorType(ctx context.Context, t string) context.Context {
	return context.WithValue(ctx, keyActorType, t)
}

func ActorType(ctx context.Context) string { return str(ctx, keyActorType) }

// WithModule menandai modul pemilik log line (mis. "shipping").
func WithModule(ctx context.Context, m string) context.Context {
	return context.WithValue(ctx, keyModule, m)
}

func str(ctx context.Context, k ctxKey) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(k).(string); ok {
		return v
	}
	return ""
}

// From mengambil logger dasar dari context bila ada (disetel middleware).
func From(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
			return l
		}
	}
	return slog.Default()
}

type loggerCtxKey struct{}

var loggerKey = loggerCtxKey{}

// WithLogger menaruh logger ber-konteks ke context.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// WithContext mengembalikan logger dengan seluruh korelasi context terpasang.
func WithContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	attrs := make([]any, 0, 8)
	if v := RequestID(ctx); v != "" {
		attrs = append(attrs, slog.String("request_id", v))
	}
	if v := TraceID(ctx); v != "" {
		attrs = append(attrs, slog.String("trace_id", v))
	}
	if v := UserID(ctx); v != "" {
		attrs = append(attrs, slog.String("user_id", v))
	}
	if v := OrganizationID(ctx); v != "" {
		attrs = append(attrs, slog.String("organization_id", v))
	}
	if v := ActorType(ctx); v != "" {
		attrs = append(attrs, slog.String("actor_type", v))
	}
	if v := str(ctx, keyModule); v != "" {
		attrs = append(attrs, slog.String("module", v))
	}
	return base.With(attrs...)
}