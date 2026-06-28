// Package account implements an account pool for the ChatGPT web backend.
//
// Mirrors basketikun/chatgpt2api's services.account_service in spirit:
// round-robin selection, slot-tracked image concurrency, OAuth token
// refresh with aliasing, 401-eviction with deferral, and a background
// watcher that keeps tokens fresh.
//
// State is kept in memory; persistence is the responsibility of the
// storage.Backend passed to NewPool (see internal/storage).
package account

import (
	"time"
)

// Status is the lifecycle status of a single account. Values mirror the
// Chinese tags used in basketikun's storage layer (but kept in English here
// for programmatic use; the admin UI maps them).
type Status string

const (
	StatusNormal  Status = "normal"  // 正常
	StatusLimited Status = "limited" // 限流
	StatusError   Status = "error"   // 异常
	StatusDisabled Status = "disabled" // 禁用
)

// PlanType mirrors OpenAI's account tiers. Free/Plus/Pro/ProLite/Team/Enterprise.
type PlanType string

const (
	PlanFree       PlanType = "free"
	PlanPlus       PlanType = "plus"
	PlanPro        PlanType = "pro"
	PlanProLite    PlanType = "prolite"
	PlanTeam       PlanType = "team"
	PlanEnterprise PlanType = "enterprise"
)

// Account is the in-memory representation of a single ChatGPT account.
type Account struct {
	ID string `json:"id,omitempty"`

	// Credentials (sensitive — must not be exposed via admin endpoints).
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	Cookie       string `json:"cookie,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
	Email        string `json:"email,omitempty"`
	Password     string `json:"password,omitempty"` // used only when AutoRelogin is enabled

	// Identity & billing.
	Type       PlanType `json:"type"`
	SourceType string   `json:"source_type,omitempty"` // web | codex | sub2api | cpa
	Status     Status   `json:"status"`
	Proxy      string   `json:"proxy,omitempty"`

	// Quota (image generation).
	Quota            int       `json:"quota"`
	RestoreAt        time.Time `json:"restore_at,omitempty"`
	ImageQuotaUnknow bool      `json:"image_quota_unknown,omitempty"`

	// Bookkeeping.
	InvalidCount   int       `json:"invalid_count,omitempty"`
	LastInvalidAt  time.Time `json:"last_invalid_at,omitempty"`
	RefreshErr     string    `json:"refresh_err,omitempty"`
	LastUsedAt     time.Time `json:"last_used_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
}

// Available returns true when the account can serve requests of the given
// kind. status==disabled is always excluded. For images, quota must be > 0
// unless ImageQuotaUnknow is set (Codex / Plus plans we can't measure).
func (a *Account) Available(isImage bool) bool {
	if a == nil {
		return false
	}
	if a.Status == StatusDisabled || a.Status == StatusError {
		return false
	}
	if isImage {
		if a.ImageQuotaUnknow {
			return true
		}
		return a.Quota > 0
	}
	return true
}

// Censor returns a copy with sensitive fields redacted, suitable for admin
// listings and JSON responses.
func (a *Account) Censor() *Account {
	if a == nil {
		return nil
	}
	cp := *a
	cp.AccessToken = redact(cp.AccessToken)
	cp.RefreshToken = redact(cp.RefreshToken)
	cp.Cookie = redact(cp.Cookie)
	cp.Password = ""
	return &cp
}

// redact keeps the first 6 chars and replaces the rest with "***".
func redact(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return "***"
	}
	return s[:6] + "***"
}
