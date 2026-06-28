// Package auth implements the API-key middleware used by the server.
//
// Keys are stored as SHA-256 hashes; the raw key is never persisted. The
// constant-time compare prevents timing attacks on hash lookup. The legacy
// master key (config.AuthKey) is also recognised as an admin identity so
// bootstrapping a fresh deployment doesn't require first creating a key.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// Role is the privilege level associated with a key.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// Key is the persisted metadata for a single API key. KeyHash is the
// hex-encoded SHA-256 of the raw key; the raw key is never stored.
type Key struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Role       Role      `json:"role"`
	KeyHash    string    `json:"key_hash"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
}

// Identity is what middlewares attach to a request after a successful auth.
type Identity struct {
	ID   string
	Name string
	Role Role
}

// IsAdmin returns true when the identity has administrative privileges.
func (i Identity) IsAdmin() bool { return i != Identity{} && i.Role == RoleAdmin }

// hashKey returns the canonical hex SHA-256 hash used for storage.
func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// compareHash runs constant-time hash comparison.
func compareHash(stored, candidate string) bool {
	if len(stored) != len(candidate) {
		return false
	}
	return hmac.Equal([]byte(stored), []byte(candidate))
}

// NewRandomKey returns a freshly minted raw key in the form sk-<base64>.
// The caller is responsible for surfacing the raw key to the user exactly
// once — only the hash is persisted.
func NewRandomKey() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return "sk-" + strings.TrimRight(base64.URLEncoding.EncodeToString(buf), "=")
}

// Service is the in-memory key registry. Persistence is delegated to a
// storage.Backend via LoadKeys / SaveKeys; the service holds the mutable
// authoritative list and the lock.
type Service struct {
	mu     sync.Mutex
	keys   []Key
	master string // legacy admin key from config (optional)
}

func NewService(masterKey string) *Service {
	return &Service{master: masterKey}
}

// LoadKeys replaces the in-memory list with the persisted one. Call once
// at startup before serving traffic.
func (s *Service) LoadKeys(keys []Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = append([]Key(nil), keys...)
}

// Keys returns a snapshot of the public view (no hashes) for admin listing.
func (s *Service) Keys() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Key, len(s.keys))
	for i, k := range s.keys {
		k.KeyHash = ""
		out[i] = k
	}
	return out
}

// SaveKeysSnapshot returns the full list (with hashes) for persistence.
func (s *Service) SaveKeysSnapshot() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Key, len(s.keys))
	copy(out, s.keys)
	return out
}

// Authenticate matches the raw key against the master key first, then the
// hashed registry. Returns the matched identity or nil.
func (s *Service) Authenticate(raw string) *Identity {
	if raw == "" {
		return nil
	}
	// Master key (legacy admin path) — constant-time compare.
	if s.master != "" && subtle.ConstantTimeCompare([]byte(raw), []byte(s.master)) == 1 {
		return &Identity{ID: "admin", Name: "管理员密钥", Role: RoleAdmin}
	}
	h := hashKey(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.keys {
		if !s.keys[i].Enabled {
			continue
		}
		if compareHash(s.keys[i].KeyHash, h) {
			s.keys[i].LastUsedAt = time.Now()
			id := Identity{ID: s.keys[i].ID, Name: s.keys[i].Name, Role: s.keys[i].Role}
			return &id
		}
	}
	return nil
}

// CreateKey mints a new key, appends to the in-memory list, and returns the
// public view + raw key (raw is exposed exactly once).
func (s *Service) CreateKey(role Role, name string) (public Key, raw string, err error) {
	if role != RoleAdmin && role != RoleUser {
		return Key{}, "", errors.New("auth: invalid role")
	}
	if name == "" {
		if role == RoleAdmin {
			name = "管理员密钥"
		} else {
			name = "普通用户"
		}
	}
	raw = NewRandomKey()
	now := time.Now()
	id := randomID()

	s.mu.Lock()
	defer s.mu.Unlock()
	k := Key{
		ID:        id,
		Name:      name,
		Role:      role,
		KeyHash:   hashKey(raw),
		Enabled:   true,
		CreatedAt: now,
	}
	s.keys = append(s.keys, k)
	public = k
	public.KeyHash = ""
	return public, raw, nil
}

// DeleteKey removes a key by id. Returns false if not found.
func (s *Service) DeleteKey(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.keys {
		if s.keys[i].ID == id {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			return true
		}
	}
	return false
}

// SetEnabled toggles a key's enabled state.
func (s *Service) SetEnabled(id string, enabled bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.keys {
		if s.keys[i].ID == id {
			s.keys[i].Enabled = enabled
			return true
		}
	}
	return false
}

func randomID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
