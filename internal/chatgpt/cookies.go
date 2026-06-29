package chatgpt

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

type browserCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type sessionResponse struct {
	AccessToken string `json:"accessToken"`
}

type Credentials struct {
	AccessToken string
	AccountID   string
	DeviceID    string
	Cookie      string
}

// OptionalCookieHeader returns the Cookie header from path, or "" on error.
func OptionalCookieHeader(path string) string {
	header, err := LoadCookieHeader(path)
	if err != nil {
		return ""
	}
	return header
}

func LoadCookieHeader(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var cookies []browserCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return "", err
	}
	if len(cookies) == 0 {
		return "", fmt.Errorf("no cookies in %s", path)
	}

	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; "), nil
}

func cookieValue(cookies []browserCookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func ResolveCredentials(accessToken, accountID, cookiesFile string) (Credentials, error) {
	creds := Credentials{
		AccessToken: accessToken,
		AccountID:   accountID,
	}
	if cookiesFile == "" {
		return creds, nil
	}

	data, err := os.ReadFile(cookiesFile)
	if err != nil {
		if creds.AccessToken != "" {
			return creds, nil
		}
		return creds, fmt.Errorf("read cookies file: %w", err)
	}

	var cookies []browserCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return creds, fmt.Errorf("parse cookies file: %w", err)
	}

	creds.Cookie = strings.Join(func() []string {
		parts := make([]string, 0, len(cookies))
		for _, c := range cookies {
			parts = append(parts, c.Name+"="+c.Value)
		}
		return parts
	}(), "; ")

	if creds.AccountID == "" {
		creds.AccountID = cookieValue(cookies, "_account")
	}
	creds.DeviceID = cookieValue(cookies, "oai-did")

	if creds.AccessToken != "" {
		return creds, nil
	}

	token, err := fetchAccessToken(creds.Cookie, creds.DeviceID)
	if err != nil {
		return creds, err
	}
	creds.AccessToken = token
	return creds, nil
}

func fetchAccessToken(cookieHeader, deviceID string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/auth/session", nil)
	if err != nil {
		return "", err
	}
	fp := httpclient.NewFingerprint()
	if deviceID != "" {
		fp.DeviceID = deviceID
	}
	h := make(http.Header)
	h.Set("Accept", mimeApplicationJSON)
	h.Set("Referer", baseURL+"/")
	h.Set("Origin", baseURL)
	if fp.UserAgent != "" {
		h.Set("User-Agent", fp.UserAgent)
	}
	if fp.DeviceID != "" {
		h.Set("Oai-Device-Id", fp.DeviceID)
	}
	h.Set("Cookie", cookieHeader)
	req.Header = h

	// Stdlib client works better here — the TLS-impersonating transport often
	// receives Cloudflare challenge pages on /api/auth/session (see sessionDo).
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("session endpoint %s", SanitizeUpstreamError(resp.StatusCode, string(body)))
	}

	var session sessionResponse
	if err := json.Unmarshal(body, &session); err != nil {
		return "", err
	}
	if session.AccessToken == "" {
		return "", fmt.Errorf("session response missing accessToken")
	}
	return session.AccessToken, nil
}