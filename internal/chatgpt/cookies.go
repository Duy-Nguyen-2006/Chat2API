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

	token, err := fetchAccessToken(creds.Cookie)
	if err != nil {
		return creds, err
	}
	creds.AccessToken = token
	return creds, nil
}

func fetchAccessToken(cookieHeader string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/auth/session", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	fp := httpclient.NewFingerprint()
	req.Header.Set("User-Agent", fp.UserAgent)
	req.Header.Set("Cookie", cookieHeader)

	// Use the TLS-impersonating client so the session endpoint isn't blocked
	// by Cloudflare when cookies are the only credential.
	var doer httpclient.Doer
	opts := httpclient.DefaultOptions()
	opts.TimeoutSeconds = 30
	c, err := httpclient.New(opts)
	if err != nil {
		doer = http.DefaultClient
	} else {
		doer = c
	}
	resp, err := doer.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("session endpoint HTTP %d: %s", resp.StatusCode, string(body))
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