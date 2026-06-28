package account

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

// stubDoer is a minimal httpclient.Doer for offline tests.
type stubDoer struct {
	resp *http.Response
	err  error
	got  *http.Request
}

func (s *stubDoer) Do(req *http.Request) (*http.Response, error) {
	s.got = req
	return s.resp, s.err
}

func TestRefreshAccessToken_Success(t *testing.T) {
	body := `{"access_token":"new-tok","refresh_token":"new-rt","expires_in":3600}`
	d := &stubDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		},
	}
	ts, err := RefreshAccessToken(context.Background(), d, "rt-1")
	if err != nil {
		t.Fatal(err)
	}
	if ts.AccessToken != "new-tok" || ts.RefreshToken != "new-rt" {
		t.Errorf("got %+v", ts)
	}
	if ts.ExpiresIn != 3600 {
		t.Errorf("expires_in: %d", ts.ExpiresIn)
	}
	// Verify the request was correctly formed.
	if d.got == nil {
		t.Fatal("request not captured")
	}
	if d.got.Method != http.MethodPost {
		t.Errorf("method: %s", d.got.Method)
	}
	if d.got.URL.String() != OAuthTokenURL {
		t.Errorf("url: %s", d.got.URL)
	}
	if d.got.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("content-type: %s", d.got.Header.Get("Content-Type"))
	}
	body2, _ := io.ReadAll(d.got.Body)
	form := string(body2)
	for _, want := range []string{"grant_type=refresh_token", "refresh_token=rt-1", "client_id=" + OAuthClientID} {
		if !strings.Contains(form, want) {
			t.Errorf("form missing %q: %s", want, form)
		}
	}
}

func TestRefreshAccessToken_Empty(t *testing.T) {
	if _, err := RefreshAccessToken(context.Background(), &stubDoer{}, ""); err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefreshAccessToken_HTTPError(t *testing.T) {
	d := &stubDoer{
		resp: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`)),
			Header:     http.Header{},
		},
	}
	_, err := RefreshAccessToken(context.Background(), d, "rt-1")
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("got %v", err)
	}
}

func TestRefreshAccessToken_TransportError(t *testing.T) {
	d := &stubDoer{err: io.ErrUnexpectedEOF}
	_, err := RefreshAccessToken(context.Background(), d, "rt-1")
	if err == nil {
		t.Error("expected error")
	}
}

func TestRefreshAccessToken_MissingAccessToken(t *testing.T) {
	d := &stubDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"refresh_token":"x"}`)),
			Header:     http.Header{},
		},
	}
	_, err := RefreshAccessToken(context.Background(), d, "rt-1")
	if err == nil || !strings.Contains(err.Error(), "access_token") {
		t.Errorf("got %v", err)
	}
}

func TestRefreshAccessToken_BadJSON(t *testing.T) {
	d := &stubDoer{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("not json")),
			Header:     http.Header{},
		},
	}
	_, err := RefreshAccessToken(context.Background(), d, "rt-1")
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestNeedsRefresh(t *testing.T) {
	cases := []struct {
		name  string
		exp   int64
		force bool
		want  bool
	}{
		{"unknown-exp", 0, false, false},
		{"expired", time.Now().Add(-time.Hour).Unix(), false, false}, // <=0 ⇒ false
		{"very-fresh", time.Now().Add(48 * time.Hour).Unix(), false, false},
		{"about-to-expire", time.Now().Add(1 * time.Hour).Unix(), false, true},
		{"force", 0, true, true},
	}
	for _, c := range cases {
		tok := makeJWT(t, map[string]any{"exp": c.exp})
		if got := NeedsRefresh(tok, c.force); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRefreshAccessToken_NilDoerBuildsRealClient(t *testing.T) {
	// When doer is nil the function should fall back to a real httpclient.
	// We don't want this test to actually hit the network, so use an
	// already-cancelled context to fail fast.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RefreshAccessToken(ctx, nil, "rt-1"); err == nil {
		t.Error("expected error with cancelled context")
	}
}

// Sanity: ensure httpclient.Doer interface still matches our stub.
var _ httpclient.Doer = (*stubDoer)(nil)

// Marshal helper to silence unused-import warnings if anyone refactors.
var _ = json.Marshal