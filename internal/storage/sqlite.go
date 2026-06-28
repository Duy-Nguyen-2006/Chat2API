package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/auth"

	_ "modernc.org/sqlite" // pure-Go driver, registers itself
)

// SQLiteBackend persists the pool and key registry to a single-file SQLite
// database (modernc.org/sqlite — no cgo). Two tables (accounts, auth_keys)
// hold the same JSON payloads as the JSON backend so the storage contract
// is implementation-agnostic.
type SQLiteBackend struct {
	mu        sync.Mutex
	db        *sql.DB
	path      string
	createdAt time.Time
}

// NewSQLiteBackend opens (or creates) the SQLite database at path. The
// parent directory must exist; pass ":memory:" for a transient backend
// (useful in tests).
func NewSQLiteBackend(path string) (*SQLiteBackend, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open sqlite %s: %w", path, err)
	}
	b := &SQLiteBackend{db: db, path: path, createdAt: time.Now()}
	if err := b.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return b, nil
}

// Close releases the database handle. Safe to call multiple times.
func (b *SQLiteBackend) Close() error {
	if b.db == nil {
		return nil
	}
	err := b.db.Close()
	b.db = nil
	return err
}

func (b *SQLiteBackend) migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			access_token TEXT PRIMARY KEY,
			payload      TEXT NOT NULL,
			updated_at   INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS auth_keys (
			id      TEXT PRIMARY KEY,
			payload TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := b.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("storage: migrate sqlite: %w", err)
		}
	}
	return nil
}

// LoadAccounts returns every row from the accounts table.
func (b *SQLiteBackend) LoadAccounts() ([]*account.Account, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	rows, err := b.db.Query(`SELECT payload FROM accounts`)
	if err != nil {
		return nil, fmt.Errorf("storage: query accounts: %w", err)
	}
	defer rows.Close()
	out := make([]*account.Account, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var a account.Account
		if err := json.Unmarshal([]byte(payload), &a); err != nil {
			return nil, fmt.Errorf("storage: parse account: %w", err)
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// SaveAccounts replaces the accounts table in a single transaction.
func (b *SQLiteBackend) SaveAccounts(accounts []*account.Account) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM accounts`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO accounts (access_token, payload, updated_at) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, a := range accounts {
		if a.AccessToken == "" {
			continue
		}
		payload, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, a.AccessToken, string(payload), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadAuthKeys returns every key.
func (b *SQLiteBackend) LoadAuthKeys() ([]auth.Key, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	rows, err := b.db.Query(`SELECT payload FROM auth_keys`)
	if err != nil {
		return nil, fmt.Errorf("storage: query keys: %w", err)
	}
	defer rows.Close()
	out := make([]auth.Key, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var k auth.Key
		if err := json.Unmarshal([]byte(payload), &k); err != nil {
			return nil, fmt.Errorf("storage: parse key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// SaveAuthKeys replaces the keys table.
func (b *SQLiteBackend) SaveAuthKeys(keys []auth.Key) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_keys`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO auth_keys (id, payload, updated_at) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, k := range keys {
		if k.ID == "" {
			continue
		}
		payload, err := json.Marshal(k)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, k.ID, string(payload), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// HealthCheck returns a SQLite ping result and table row counts.
func (b *SQLiteBackend) HealthCheck() map[string]any {
	out := map[string]any{"ok": true}
	if b.db == nil {
		out["ok"] = false
		out["error"] = "db closed"
		return out
	}
	if err := b.db.Ping(); err != nil {
		out["ok"] = false
		out["error"] = err.Error()
		return out
	}
	for _, t := range []string{"accounts", "auth_keys"} {
		var n int
		if err := b.db.QueryRow(`SELECT COUNT(*) FROM ` + t).Scan(&n); err != nil {
			out[t] = "err: " + err.Error()
		} else {
			out[t] = n
		}
	}
	return out
}

// Info describes the backend.
func (b *SQLiteBackend) Info() map[string]any {
	return map[string]any{
		"name":       "sqlite",
		"type":       "sqlite",
		"path":       b.path,
		"created_at": b.createdAt,
	}
}

// ErrBackendClosed is returned when an operation hits a closed database.
var ErrBackendClosed = errors.New("storage: backend closed")
