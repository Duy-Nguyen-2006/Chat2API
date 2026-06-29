package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
)

// Loader persists and restores the account pool. The on-disk format mirrors
// basketikun's accounts.json: an array of Account objects with sensitive
// fields stored alongside opaque identifiers.
type Loader struct {
	path string
}

// NewLoader returns a Loader bound to path. The file is created on demand;
// missing files are not an error.
func NewLoader(path string) *Loader {
	return &Loader{path: path}
}

// Load reads the persisted pool. Returns an empty slice if the file doesn't
// exist; any other I/O or parse error is returned.
func (l *Loader) Load() ([]*Account, error) {
	if l.path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("account: read %s: %w", l.path, err)
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("account: parse %s: %w", l.path, err)
	}
	out := make([]*Account, 0, len(raw))
	for i, r := range raw {
		var a Account
		if err := json.Unmarshal(r, &a); err != nil {
			return nil, fmt.Errorf("account: entry %d in %s: %w", i, l.path, err)
		}
		out = append(out, &a)
	}
	return out, nil
}

// Save writes the pool atomically (write tmp, rename).
func (l *Loader) Save(accounts []*Account) error {
	if l.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), "accounts-*.json.tmp")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(accounts); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), l.path)
}

// MigrateFromCookies converts a single cookies_<n>.json file into an Account
// suitable for adding to the pool. Used as a fallback when accounts.json
// is absent but the legacy single-account flow is configured.
//
// The AccountID / Email / DeviceID are extracted from the cookies. The
// access_token is fetched via /api/auth/session (requires cookies to be
// valid; errors are returned to the caller).
func MigrateFromCookies(cookiesFile string) (*Account, error) {
	return MigrateFromCookiesWithToken(cookiesFile, "")
}

// MigrateFromCookiesWithToken builds an Account from a cookie export. When
// accessToken is non-empty (e.g. CHATGPT_ACCESS_TOKEN from .env) the session
// endpoint is skipped — useful when Cloudflare blocks /api/auth/session.
func MigrateFromCookiesWithToken(cookiesFile, accessToken string) (*Account, error) {
	// We use the chatgpt package helpers — avoids duplicating cookie parsing
	// in this package and lets the caller refresh the access token via the
	// same TLS-impersonating client the rest of the app uses.
	creds, err := chatgpt.ResolveCredentials(accessToken, "", cookiesFile)
	if err != nil {
		return nil, fmt.Errorf("account: resolve cookies: %w", err)
	}
	if creds.AccessToken == "" {
		return nil, fmt.Errorf("account: cookies %s produced no access token", cookiesFile)
	}
	acc := &Account{
		AccessToken: creds.AccessToken,
		AccountID:   creds.AccountID,
		DeviceID:    creds.DeviceID,
		Cookie:      creds.Cookie,
		CookiesFile: cookiesFile,
		Status:      StatusNormal,
		SourceType:  "cookies",
		CreatedAt:   time.Now(),
	}
	claims := DecodeJWT(creds.AccessToken)
	if claims.Email() != "" {
		acc.Email = claims.Email()
	}
	if claims.ChatGPTAccountID() != "" && acc.AccountID == "" {
		acc.AccountID = claims.ChatGPTAccountID()
	}
	return acc, nil
}

// DisplayName produces a human-friendly identifier for admin UI / logs:
// prefer email, fall back to account_id prefix, then to a short token hash.
func DisplayName(a *Account) string {
	if a == nil {
		return "<nil>"
	}
	if a.Email != "" {
		return a.Email
	}
	if a.AccountID != "" {
		return "acct_" + a.AccountID
	}
	if len(a.AccessToken) >= 8 {
		return "tok_" + strings.ToLower(a.AccessToken[:8])
	}
	return "<empty>"
}
