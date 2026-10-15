package shared

import (
	"time"

	"github.com/google/uuid"
)

// AuditFields disematkan pada entity yang dapat diaudit. organization_id
// SELALU berasal dari context tenant, tidak pernah dari input pengguna.
type AuditFields struct {
	OrganizationID uuid.UUID  `json:"organization_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CreatedBy      *uuid.UUID `json:"created_by,omitempty"`
	UpdatedBy      *uuid.UUID `json:"updated_by,omitempty"`
}

// Versioned menyediakan optimistic locking. Repository wajib memakai kolom
// version pada klausa WHERE saat UPDATE, dan gagal bila tidak ada baris cocok.
type Versioned struct {
	Version int `json:"version"`
}

// ActorType membedakan perubahan oleh manusia, sistem, atau AI.
// Dipakai di audit_logs; aksi AI tidak pernah menyamar sebagai manusia.
type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorSystem ActorType = "system"
	ActorAI     ActorType = "ai"
	ActorAPI    ActorType = "api_key"
)
