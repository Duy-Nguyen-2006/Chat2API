package account

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

// OAuth config — kept as constants so they match the ChatGPT web client.
// Client ID is the public OAuth app id used by chatgpt.com.
const (
	OAuthTokenURL  = "https://auth.openai.com/oauth/token"
	OAuthClientID  = "app_2SKx67EdpoN0G6j64rFvigXD"
	OAuthUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
)

// TokenSet is the result of a successful OAuth refresh or password login.
type TokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

// RefreshAccessToken exchanges a refresh_token for a new access_token at the
// OpenAI OAuth endpoint. Returns ErrInvalidToken if refresh_token is empty.
func RefreshAccessToken(ctx context.Context, doer httpclient.Doer, refreshToken string) (*TokenSet, error) {
	if refreshToken == "" {
		return nil, ErrInvalidToken
	}
	if doer == nil {
		var err error
		doer, err = httpclient.New(httpclient.DefaultOptions())
		if err != nil {
			return nil, fmt.Errorf("account: build oauth client: %w", err)
		}
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", OAuthClientID)
	form.Set("scope", "openid profile email")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", OAuthUserAgent)
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")

	resp, err := doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("account: oauth POST: %w", err)
	}
	defer resp.Body.Close()

	body, _ := readAllLimited(resp.Body, 256<<10)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account: oauth HTTP %d: %s", resp.StatusCode, string(body))
	}

	var ts TokenSet
	if err := json.Unmarshal(body, &ts); err != nil {
		return nil, fmt.Errorf("account: oauth parse: %w", err)
	}
	if ts.AccessToken == "" {
		return nil, fmt.Errorf("account: oauth response missing access_token")
	}
	return &ts, nil
}

// NeedsRefresh returns true when the access token should be refreshed before
// use. We refresh early (24h before expiry) to absorb clock skew and avoid
// mid-request 401s. force bypasses the expiry check.
func NeedsRefresh(accessToken string, force bool) bool {
	if force {
		return true
	}
	secs := SecondsUntilExpiry(accessToken)
	if secs <= 0 {
		return false // unknown / unparsable — let the caller try, evict on 401
	}
	// 24h headroom, matches basketikun's _ACCESS_TOKEN_REFRESH_SKEW_SECONDS.
	return secs < 24*60*60
}

// readAllLimited drains at most n bytes from r. Errors are swallowed; an
// empty result still returns the bytes that were read.
func readAllLimited(r interface {
	Read(p []byte) (int, error)
}, n int64) ([]byte, error) {
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		if int64(len(buf)) >= n {
			return buf, nil
		}
		want := int64(cap(tmp))
		if int64(len(buf))+want > n {
			want = n - int64(len(buf))
		}
		read, err := r.Read(tmp[:want])
		if read > 0 {
			buf = append(buf, tmp[:read]...)
		}
		if err != nil {
			return buf, nil
		}
	}
}

// TimeUntilRefresh computes the time until the given token should be refreshed.
func TimeUntilRefresh(t time.Time) time.Duration {
	return time.Until(t)
}
