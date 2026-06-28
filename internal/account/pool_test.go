package account

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

func newTestPool(t *testing.T) *Pool {
	t.Helper()
	doer, err := httpclient.New(httpclient.DefaultOptions())
	if err != nil {
		t.Skipf("tls-client unavailable in test env: %v", err)
	}
	return NewPool(doer, PoolOptions{ImageConcurrency: 2, AutoRemoveInvalid: false})
}

func TestPoolRoundRobin(t *testing.T) {
	p := newTestPool(t)
	for i := 0; i < 3; i++ {
		p.Upsert(&Account{AccessToken: "tok" + string(rune('A'+i)), Status: StatusNormal})
	}
	seen := map[string]int{}
	for i := 0; i < 9; i++ {
		acc, err := p.GetTextToken(context.Background(), nil)
		if err != nil {
			t.Fatalf("GetTextToken: %v", err)
		}
		seen[acc.AccessToken]++
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct tokens across picks, got %d", len(seen))
	}
	// Every token should be picked at least once — round-robin guarantees
	// coverage, but distribution may vary slightly between runs because the
	// selector iterates over a map and picks tokens[len(p.index)%len(tokens)].
	for tok, n := range seen {
		if n < 1 {
			t.Errorf("token %s never picked", tok)
		}
	}
}

func TestPoolExcludesErroredAndDisabled(t *testing.T) {
	p := newTestPool(t)
	p.Upsert(&Account{AccessToken: "ok1", Status: StatusNormal})
	p.Upsert(&Account{AccessToken: "bad", Status: StatusDisabled})
	p.Upsert(&Account{AccessToken: "err", Status: StatusError})

	for i := 0; i < 5; i++ {
		acc, err := p.GetTextToken(context.Background(), nil)
		if err != nil {
			t.Fatalf("GetTextToken: %v", err)
		}
		if acc.AccessToken == "bad" || acc.AccessToken == "err" {
			t.Fatalf("returned excluded token: %s", acc.AccessToken)
		}
	}
}

func TestPoolExclusionList(t *testing.T) {
	p := newTestPool(t)
	p.Upsert(&Account{AccessToken: "a", Status: StatusNormal})
	p.Upsert(&Account{AccessToken: "b", Status: StatusNormal})
	for i := 0; i < 6; i++ {
		acc, err := p.GetTextToken(context.Background(), []string{"a"})
		if err != nil {
			t.Fatalf("GetTextToken: %v", err)
		}
		if acc.AccessToken != "b" {
			t.Fatalf("expected only token 'b', got %q", acc.AccessToken)
		}
	}
}

func TestPoolImageSlotRelease(t *testing.T) {
	p := newTestPool(t)
	p.Upsert(&Account{AccessToken: "img", Status: StatusNormal, Quota: 1})
	// Acquire both slots.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	first, err := p.GetImageToken(ctx, "")
	if err != nil {
		t.Fatalf("first GetImageToken: %v", err)
	}
	if first.AccessToken != "img" {
		t.Fatalf("expected token 'img', got %q", first.AccessToken)
	}
	// Release the slot synchronously, then the next acquire should succeed
	// without needing the cond.Wait path.
	p.ReleaseImageSlot("img")
	second, err := p.GetImageToken(ctx, "")
	if err != nil {
		t.Fatalf("second GetImageToken: %v", err)
	}
	if second.AccessToken != "img" {
		t.Fatalf("expected token 'img', got %q", second.AccessToken)
	}
	p.ReleaseImageSlot("img")
}

func TestPoolImageSlotBlocksUntilRelease(t *testing.T) {
	p := newTestPool(t)
	// Concurrency=1 means second acquire must wait.
	p.mu.Lock()
	p.opts.ImageConcurrency = 1
	p.mu.Unlock()
	p.Upsert(&Account{AccessToken: "img", Status: StatusNormal, Quota: 1})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	first, err := p.GetImageToken(ctx, "")
	if err != nil {
		t.Fatalf("first GetImageToken: %v", err)
	}
	if first.AccessToken != "img" {
		t.Fatalf("expected token 'img', got %q", first.AccessToken)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		p.ReleaseImageSlot("img")
	}()
	second, err := p.GetImageToken(ctx, "")
	if err != nil {
		t.Fatalf("second GetImageToken: %v", err)
	}
	if second.AccessToken != "img" {
		t.Fatalf("expected token 'img', got %q", second.AccessToken)
	}
	p.ReleaseImageSlot("img")
}

func TestPoolMarkInvalidDeferral(t *testing.T) {
	p := newTestPool(t)
	acc := &Account{AccessToken: "tok", Status: StatusNormal, CreatedAt: time.Now().Add(-time.Hour)}
	p.Upsert(acc)

	// First failure should defer (invalid_count==1).
	if removed := p.MarkInvalid("tok", nil); removed {
		t.Fatal("expected first invalidation to defer")
	}
	if acc.Status != StatusNormal {
		t.Fatalf("expected status to remain normal, got %s", acc.Status)
	}
	// Second failure should mark the account errored (auto-remove disabled).
	if removed := p.MarkInvalid("tok", nil); removed {
		t.Fatal("expected no removal with AutoRemoveInvalid=false")
	}
	if acc.Status != StatusError {
		t.Fatalf("expected status=error, got %s", acc.Status)
	}
}

func TestPoolApplyRefreshedToken(t *testing.T) {
	p := newTestPool(t)
	p.Upsert(&Account{AccessToken: "old", Status: StatusNormal, RefreshToken: "r"})

	ts := &TokenSet{AccessToken: "new", RefreshToken: "r2", ExpiresIn: 3600}
	newTok, rotated := p.ApplyRefreshedToken("old", ts)
	if !rotated || newTok != "new" {
		t.Fatalf("expected rotation to 'new', got %q rotated=%v", newTok, rotated)
	}
	// Resolve the old token should now follow the alias chain.
	if got := p.ResolveToken("old"); got != "new" {
		t.Fatalf("expected ResolveToken(old)='new', got %q", got)
	}
}

func TestLoaderRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	l := NewLoader(path)

	original := []*Account{
		{AccessToken: "t1", Email: "a@example.com", Status: StatusNormal, CreatedAt: time.Now()},
		{AccessToken: "t2", Email: "b@example.com", Status: StatusLimited, Quota: 0},
	}
	if err := l.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected accounts.json to exist: %v", err)
	}
	loaded, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(loaded))
	}
	if loaded[0].AccessToken != "t1" || loaded[0].Email != "a@example.com" {
		t.Errorf("first account mismatch: %+v", loaded[0])
	}
	if loaded[1].Status != StatusLimited {
		t.Errorf("second account status mismatch: %s", loaded[1].Status)
	}
}

func TestJWTDecodeRoundTrip(t *testing.T) {
	// Header.Payload.Signature — base64url-encoded payload below decodes to
	// {"exp": 9999999999, "https://api.openai.com/profile": {"email": "x@y"}}.
	const tok = "eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjk5OTk5OTk5OTksImh0dHBzOi8vYXBpLm9wZW5haS5jb20vcHJvZmlsZSI6eyJlbWFpbCI6InhAeSJ9fQ.signature"
	c := DecodeJWT(tok)
	if c.ExpUnix() != 9999999999 {
		t.Errorf("exp mismatch: %d", c.ExpUnix())
	}
	if c.Email() != "x@y" {
		t.Errorf("email mismatch: %q", c.Email())
	}
}
