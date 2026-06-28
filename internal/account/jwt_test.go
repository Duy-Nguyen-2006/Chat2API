package account

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// makeJWT builds a synthetic JWT with the given JSON payload.
func makeJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	p := base64.RawURLEncoding.EncodeToString(body)
	s := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	return h + "." + p + "." + s
}

func TestDecodeJWT_Claims(t *testing.T) {
	tok := makeJWT(t, map[string]any{
		"exp": int64(2000000000),
		"iat": int64(1900000000),
		"https://api.openai.com/profile": map[string]string{
			"email": "alice@example.com",
		},
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acc-123",
		},
	})
	c := DecodeJWT(tok)
	if c.Exp != 2000000000 {
		t.Errorf("exp: %d", c.Exp)
	}
	if c.Email() != "alice@example.com" {
		t.Errorf("email: %q", c.Email())
	}
	if c.ChatGPTAccountID() != "acc-123" {
		t.Errorf("acc id: %q", c.ChatGPTAccountID())
	}
}

func TestDecodeJWT_Malformed(t *testing.T) {
	cases := []string{
		"",
		"abc",
		"abc.def",
		"!!!.???.###",
	}
	for _, c := range cases {
		_ = DecodeJWT(c) // must not panic
	}
}

func TestSecondsUntilExpiry(t *testing.T) {
	now := time.Now().Unix()
	tok := makeJWT(t, map[string]any{"exp": now + 120})
	sec := SecondsUntilExpiry(tok)
	if sec < 100 || sec > 130 {
		t.Errorf("expected ~120s, got %d", sec)
	}
}

func TestSecondsUntilExpiry_NoExp(t *testing.T) {
	tok := makeJWT(t, map[string]any{"iat": time.Now().Unix()})
	if got := SecondsUntilExpiry(tok); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestIsExpired(t *testing.T) {
	future := makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	if IsExpired(future) {
		t.Error("future token should not be expired")
	}
	past := makeJWT(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
	if !IsExpired(past) {
		t.Error("past token should be expired")
	}
	noExp := makeJWT(t, map[string]any{})
	if !IsExpired(noExp) {
		t.Error("token without exp should be considered expired")
	}
}

func TestFormatExpiry(t *testing.T) {
	tok := makeJWT(t, map[string]any{"exp": int64(1234567890)})
	if got := FormatExpiry(tok); got != "1234567890" {
		t.Errorf("format: %q", got)
	}
	if got := FormatExpiry("not-a-jwt"); got != "" {
		t.Errorf("bad token: %q", got)
	}
}

func TestJWTClaims_HelperSafety(t *testing.T) {
	c := jwtClaims{}
	if got := c.Email(); got != "" {
		t.Errorf("email: %q", got)
	}
	if got := c.ChatGPTAccountID(); got != "" {
		t.Errorf("acc id: %q", got)
	}
	c.AuthClaim = map[string]any{"chatgpt_account_id": 42}
	if got := c.ChatGPTAccountID(); got != "" {
		t.Errorf("non-string should yield empty, got %q", got)
	}
}

func TestDecodeJWT_PaddedPayload(t *testing.T) {
	// Real tokens sometimes need padding (URLEncoding variant).
	rawJSON := `{"exp":9999999999}`
	b := base64.URLEncoding.EncodeToString([]byte(rawJSON))
	b = strings.TrimRight(b, "=")
	tok := "h." + b + ".s"
	c := DecodeJWT(tok)
	if c.Exp != 9999999999 {
		t.Errorf("expected exp 9999999999, got %d", c.Exp)
	}
}