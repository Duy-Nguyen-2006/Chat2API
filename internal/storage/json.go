package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/auth"
)

// JSONBackend persists the pool and key registry to two JSON files. Writes
// are atomic (write tmp file, rename) so a crash mid-write can't corrupt the
// canonical data. This is the default backend and the one used in tests.
type JSONBackend struct {
	dir          string
	accountsPath string
	authKeysPath string
	createdAt    time.Time
}

// NewJSONBackend binds to dir (created on first save). accountsFile and
// authKeysFile may be absolute or relative to dir; empty strings default to
// accounts.json and auth_keys.json.
func NewJSONBackend(dir, accountsFile, authKeysFile string) *JSONBackend {
	if accountsFile == "" {
		accountsFile = "accounts.json"
	}
	if authKeysFile == "" {
		authKeysFile = "auth_keys.json"
	}
	return &JSONBackend{
		dir:          dir,
		accountsPath: filepath.Join(dir, accountsFile),
		authKeysPath: filepath.Join(dir, authKeysFile),
		createdAt:    time.Now(),
	}
}

// LoadAccounts reads accounts.json. A missing file returns an empty pool.
func (b *JSONBackend) LoadAccounts() ([]*account.Account, error) {
	data, err := os.ReadFile(b.accountsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: read accounts: %w", err)
	}
	var accounts []*account.Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("storage: parse accounts: %w", err)
	}
	return accounts, nil
}

// SaveAccounts writes accounts.json atomically.
func (b *JSONBackend) SaveAccounts(accounts []*account.Account) error {
	return atomicWriteJSON(b.accountsPath, accounts)
}

// LoadAuthKeys reads auth_keys.json. Missing file returns empty.
func (b *JSONBackend) LoadAuthKeys() ([]auth.Key, error) {
	data, err := os.ReadFile(b.authKeysPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: read auth_keys: %w", err)
	}
	var keys []auth.Key
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("storage: parse auth_keys: %w", err)
	}
	return keys, nil
}

// SaveAuthKeys writes auth_keys.json atomically.
func (b *JSONBackend) SaveAuthKeys(keys []auth.Key) error {
	return atomicWriteJSON(b.authKeysPath, keys)
}

// HealthCheck reports whether the two files exist and their sizes.
func (b *JSONBackend) HealthCheck() map[string]any {
	out := map[string]any{"ok": true}
	for _, p := range []string{b.accountsPath, b.authKeysPath} {
		st, err := os.Stat(p)
		if err != nil {
			out[p] = "missing"
			continue
		}
		out[p] = map[string]any{"size": st.Size(), "modified": st.ModTime()}
	}
	return out
}

// Info describes the backend.
func (b *JSONBackend) Info() map[string]any {
	return map[string]any{
		"name":       "json",
		"type":       "json",
		"dir":        b.dir,
		"created_at": b.createdAt,
	}
}

func atomicWriteJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
