package storage

import (
	"fmt"
	"os"
	"strings"
)

// Type enumerates the supported backends.
type Type string

const (
	TypeJSON   Type = "json"
	TypeSQLite Type = "sqlite"
)

// Config selects and parameterises a backend. Empty fields fall back to
// the implementation's defaults.
type Config struct {
	Type          Type
	DataDir       string
	AccountsFile  string
	AuthKeysFile  string
	SQLitePath    string
}

// New constructs a Backend from cfg. Unknown types yield an error.
func New(cfg Config) (Backend, error) {
	switch strings.ToLower(string(cfg.Type)) {
	case "", "json":
		dir := cfg.DataDir
		if dir == "" {
			dir = "data"
		}
		return NewJSONBackend(dir, cfg.AccountsFile, cfg.AuthKeysFile), nil
	case "sqlite":
		path := cfg.SQLitePath
		if path == "" {
			dir := cfg.DataDir
			if dir == "" {
				dir = "data"
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("storage: mkdir %s: %w", dir, err)
			}
			path = dir + "/chat2api.db"
		}
		return NewSQLiteBackend(path)
	default:
		return nil, fmt.Errorf("storage: unknown backend type %q", cfg.Type)
	}
}
