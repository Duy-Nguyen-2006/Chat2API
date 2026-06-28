package account

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// jwtClaims is the subset of OpenAI's JWT body we care about.
type jwtClaims struct {
	Exp        int64 `json:"exp"`
	Iat        int64 `json:"iat"`
	AuthClaim  map[string]any `json:"https://api.openai.com/auth"`
	Profile    map[string]string `json:"https://api.openai.com/profile"`
}

// DecodeJWT extracts the payload claims from a JWT (header.payload.signature).
// Returns an empty struct on any parse error so callers can detect invalid
// tokens without panicking.
func DecodeJWT(token string) jwtClaims {
	var c jwtClaims
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return c
	}
	payload := parts[1]
	// base64url needs padding
	if pad := len(payload) % 4; pad != 0 {
		payload += strings.Repeat("=", 4-pad)
	}
	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Some tokens use RawURLEncoding (no padding) — retry.
		data, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return c
		}
	}
	_ = json.Unmarshal(data, &c)
	return c
}

// Exp returns the JWT expiry timestamp (unix seconds) or 0 if unknown.
func (c jwtClaims) ExpUnix() int64 { return c.Exp }

// Email returns the email claim if present.
func (c jwtClaims) Email() string {
	if c.Profile == nil {
		return ""
	}
	return c.Profile["email"]
}

// ChatGPTAccountID returns the chatgpt_account_id from the auth claim.
func (c jwtClaims) ChatGPTAccountID() string {
	if c.AuthClaim == nil {
		return ""
	}
	if v, ok := c.AuthClaim["chatgpt_account_id"].(string); ok {
		return v
	}
	return ""
}

// SecondsUntilExpiry returns the seconds remaining until the JWT expires.
// Negative values mean the token is already expired.
func SecondsUntilExpiry(token string) int64 {
	c := DecodeJWT(token)
	if c.Exp == 0 {
		return 0
	}
	return c.Exp - time.Now().Unix()
}

// FormatExpiry renders the expiry as a RFC3339-ish string for logs.
func FormatExpiry(token string) string {
	c := DecodeJWT(token)
	if c.Exp == 0 {
		return ""
	}
	return strconv.FormatInt(c.Exp, 10)
}

// IsExpired returns true when the token has no exp claim or the exp is in the past.
func IsExpired(token string) bool {
	sec := SecondsUntilExpiry(token)
	return sec <= 0
}

// ErrInvalidToken is returned by RefreshAccessToken when the input is unusable.
var ErrInvalidToken = errors.New("account: token is not a valid JWT")
