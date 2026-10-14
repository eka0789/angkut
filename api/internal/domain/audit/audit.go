// Package audit mencatat "siapa melakukan apa, kapan". Tabelnya append-only —
// koreksi dilakukan dengan baris baru, bukan UPDATE.
package audit

import (
	"context"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

// Entry adalah satu catatan audit.
type Entry struct {
	OrganizationID string // kosong untuk aksi pra-organisasi (mis. login)
	ActorUserID    string // kosong bila sistem
	ActorType      shared.ActorType
	Action         string // mis. "auth.login", "shipment.cancelled"
	EntityType     string
	EntityID       string
	Changes        map[string]any // {before, after} bila relevan
	IP             string
	UserAgent      string
	RequestID      string
}

// Recorder adalah port pencatatan audit.
type Recorder interface {
	// Record menyimpan satu entri. Implementasi TIDAK boleh menggagalkan
	// operasi bisnis karena audit gagal — kegagalan dicatat ke log, bukan
	// dilempar ke pemanggil (kecuali diminta tegas oleh alur kepatuhan).
	Record(ctx context.Context, e Entry) error
}

// Nama aksi yang dipakai lintas modul. Dikumpulkan di sini supaya filter di
// panel admin dan pencarian tidak bergantung pada string yang diketik manual.
const (
	ActionRegister            = "auth.register"
	ActionLogin               = "auth.login"
	ActionLoginFailed         = "auth.login_failed"
	ActionLoginLocked         = "auth.login_locked"
	ActionLoginBlocked        = "auth.login_blocked"
	ActionLogout              = "auth.logout"
	ActionLogoutAll           = "auth.logout_all"
	ActionTokenRefresh        = "auth.token_refresh"
	ActionRefreshReuseBlocked = "auth.refresh_reuse_blocked"
	ActionEmailVerified       = "auth.email_verified"
	ActionPasswordResetReq    = "auth.password_reset_requested"
	ActionPasswordReset       = "auth.password_reset"
	ActionPasswordChanged     = "auth.password_changed"
	ActionTwoFASetup          = "auth.2fa_setup"
	ActionTwoFAEnabled        = "auth.2fa_enabled"
	ActionTwoFADisabled       = "auth.2fa_disabled"
	ActionTwoFAFailed         = "auth.2fa_failed"
	ActionRecoveryUsed        = "auth.2fa_recovery_used"
	ActionSessionRevoked      = "auth.session_revoked"
	ActionOAuthLogin          = "auth.oauth_login"

	ActionOrgCreated    = "organization.created"
	ActionOrgUpdated    = "organization.updated"
	ActionMemberInvited = "organization.member_invited"
	ActionMemberAdded   = "organization.member_added"
	ActionMemberUpdated = "organization.member_updated"
	ActionMemberRemoved = "organization.member_removed"

	// Order (TASK-026).
	ActionOrderCreated       = "order.created"
	ActionOrderStatusChanged = "order.status_changed"

	// Shipment (TASK-027/028).
	ActionShipmentCreated       = "shipment.created"
	ActionShipmentStatusChanged = "shipment.status_changed"
	ActionShipmentCanceled      = "shipment.cancelled"
	ActionShipmentLabeled       = "shipment.label_generated"
)

// New membangun entri dengan aktor manusia.
func New(action string, actorUserID, orgID string) Entry {
	return Entry{
		Action:         action,
		ActorUserID:    actorUserID,
		OrganizationID: orgID,
		ActorType:      shared.ActorUser,
	}
}

// WithEntity melampirkan entitas yang terpengaruh.
func (e Entry) WithEntity(entityType, entityID string) Entry {
	e.EntityType = entityType
	e.EntityID = entityID
	return e
}

// WithChanges melampirkan diff sebelum/sesudah.
func (e Entry) WithChanges(before, after any) Entry {
	e.Changes = map[string]any{"before": before, "after": after}
	return e
}

// WithRequest melampirkan metadata HTTP untuk penelusuran.
func (e Entry) WithRequest(ip, userAgent, requestID string) Entry {
	e.IP = ip
	e.UserAgent = userAgent
	e.RequestID = requestID
	return e
}

// AsSystem menandai entri berasal dari proses sistem (worker/scheduler).
func (e Entry) AsSystem() Entry {
	e.ActorType = shared.ActorSystem
	e.ActorUserID = ""
	return e
}

// AsAI menandai entri berasal dari aksi asisten AI — tidak pernah menyamar
// sebagai manusia.
func (e Entry) AsAI() Entry {
	e.ActorType = shared.ActorAI
	return e
}

// Event adalah nama aksi untuk audit yang tidak menempel pada satu user
// (mis. percobaan login gagal terhadap email yang tidak ada).
func Event(action string) Entry {
	return Entry{
		Action:    action,
		ActorType: shared.ActorSystem,
	}
}

// Clock memudahkan pengujian; default waktu sistem.
type Clock interface{ Now() time.Time }
