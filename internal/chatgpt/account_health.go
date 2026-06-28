package chatgpt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
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

func ProbeAccountHealth(ctx context.Context, creds Credentials) (status string, detail string) {
	if creds.AccessToken == "" {
		return "dead", "no access token"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+accountsCheckPath, nil)
	if err != nil {
		return "dead", err.Error()
	}
	client := NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)
	req.Header = client.buildHeaders(nil)

	resp, err := client.http.Do(req)
	if err != nil {
		return "dead", err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "dead", fmt.Sprintf("HTTP %d — session expired or blocked", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "dead", fmt.Sprintf("HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}

	var check accountsCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&check); err != nil {
		return "dead", "invalid response"
	}

	accessible := 0
	for key, entry := range check.Accounts {
		if key == "default" {
			continue
		}
		if entry.CanAccessWithSession {
			accessible++
		}
	}
	if accessible == 0 {
		return "dead", "no accessible workspaces"
	}
	return "alive", fmt.Sprintf("%d workspace(s) accessible", accessible)
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