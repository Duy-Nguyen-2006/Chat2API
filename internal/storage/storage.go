// Package storage abstracts the persistence layer used by the account pool
// and auth key registry. The interface mirrors basketikun's
// services.storage.base — six methods, no business logic. Implementations
// (json, sqlite) live in sibling files in this package.
package storage

import (
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/auth"
)

// Backend is the persistence contract. All methods must be safe for
// concurrent callers; implementations decide whether to serialise via locks
// or rely on the underlying engine.
type Backend interface {
	// LoadAccounts returns the full account pool. An empty pool is not an
	// error; only I/O or parse failures return a non-nil error.
	LoadAccounts() ([]*account.Account, error)
	// SaveAccounts replaces the persisted pool atomically.
	SaveAccounts(accounts []*account.Account) error
	// LoadAuthKeys returns all stored API keys (including the hashed form).
	LoadAuthKeys() ([]auth.Key, error)
	// SaveAuthKeys replaces the persisted key set atomically.
	SaveAuthKeys(keys []auth.Key) error
	// HealthCheck returns a backend-specific health snapshot.
	HealthCheck() map[string]any
	// Info returns backend metadata (name, version, etc.).
	Info() map[string]any
}

// Info is the static metadata block returned by Info().
type Info struct {
	Name      string    `json:"name"`
	Type      string    `json:"type"` // json | sqlite | postgres | git
	CreatedAt time.Time `json:"created_at"`
}
