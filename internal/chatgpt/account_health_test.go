package chatgpt

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEmailFromJWT(t *testing.T) {
	// JWT: header.payload.signature with payload carrying email claim.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/profile":{"email":"alice@example.com"}}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	tok := header + "." + payload + "." + sig

	if got := emailFromJWT(tok); got != "alice@example.com" {
		t.Errorf("email: %q", got)
	}
}

func TestEmailFromJWT_Malformed(t *testing.T) {
	cases := []string{
		"",                       // empty
		"abc",                    // single segment
		"abc.def",                // no signature
		"!!!!.!!!!.!!!!",         // invalid base64
		"aGVsbG8.notbase64.sig",  // invalid payload base64 (no padding, '==' → not url-safe)
	}
	for _, c := range cases {
		if got := emailFromJWT(c); got != "" {
			t.Errorf("emailFromJWT(%q) = %q, want empty", c, got)
		}
	}
}

func TestProbeAccountHealth_NoToken(t *testing.T) {
	status, detail := ProbeAccountHealth(context.Background(), Credentials{})
	if status != "dead" || !strings.Contains(detail, "no access token") {
		t.Errorf("got (%q,%q), want (dead, ...)", status, detail)
	}
}

func TestIsInconclusiveUpstream(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"accounts/check HTTP 403: challenge", true},
		{"accounts/check HTTP 401: unauthorized", false},
		{"accounts/check HTTP 429: rate limited", true},
		{"no access token", false},
	}
	for _, tc := range cases {
		if got := isInconclusiveUpstream(tc.msg); got != tc.want {
			t.Errorf("isInconclusiveUpstream(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}