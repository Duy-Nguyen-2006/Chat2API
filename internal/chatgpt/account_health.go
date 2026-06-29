package chatgpt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AccountStatus struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	CookiesFile  string `json:"cookies_file,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	Email        string `json:"email,omitempty"`
	Status       string `json:"status"`
	StatusDetail string `json:"status_detail,omitempty"`
	CheckedAt    string `json:"checked_at"`
}

func emailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Profile struct {
			Email string `json:"email"`
		} `json:"https://api.openai.com/profile"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.Profile.Email
}

// IsInconclusiveError reports upstream failures that do not prove the session
// is dead (e.g. Cloudflare challenge, rate limit).
func IsInconclusiveError(msg string) bool {
	return isInconclusiveUpstream(msg)
}

func isInconclusiveUpstream(msg string) bool {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "HTTP 403"),
		strings.Contains(msg, "HTTP 429"),
		strings.Contains(msg, "HTTP 502"),
		strings.Contains(msg, "HTTP 503"),
		strings.Contains(msg, "HTTP 504"):
		return true
	case strings.Contains(lower, "challenge"),
		strings.Contains(lower, "cf_chl"),
		strings.Contains(lower, "cloudflare"),
		strings.Contains(lower, "unusual activity"):
		return true
	default:
		return false
	}
}

// ProbeHealth checks whether the account can reach ChatGPT workspaces.
// Returns alive, dead, or unknown (upstream blocked — e.g. Cloudflare).
func (c *Client) ProbeHealth(ctx context.Context) (status string, detail string) {
	if c.accessToken == "" {
		return "dead", "no access token"
	}

	list, err := c.ListWorkspaces(ctx)
	if err != nil {
		msg := err.Error()
		if isInconclusiveUpstream(msg) {
			return "unknown", msg
		}
		return "dead", msg
	}

	accessible := 0
	for _, w := range list.Data {
		if w.CanAccess {
			accessible++
		}
	}
	if accessible == 0 {
		return "dead", "no accessible workspaces"
	}
	return "alive", fmt.Sprintf("%d workspace(s) accessible", accessible)
}

func ProbeAccountHealth(ctx context.Context, creds Credentials) (status string, detail string) {
	client := NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)
	return client.ProbeHealth(ctx)
}

func BuildAccountStatus(id, label, cookiesFile string, creds Credentials) AccountStatus {
	status, detail := ProbeAccountHealth(context.Background(), creds)
	email := emailFromJWT(creds.AccessToken)
	return AccountStatus{
		ID:           id,
		Label:        label,
		CookiesFile:  cookiesFile,
		AccountID:    creds.AccountID,
		Email:        email,
		Status:       status,
		StatusDetail: detail,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}