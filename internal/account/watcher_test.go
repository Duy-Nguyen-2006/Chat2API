package account

import (
	"context"
	"testing"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

// stubDoer is duplicated here (and in oauth_test.go) to keep each test
// self-contained. In Go they live in the same package so we re-use the
// existing one declared in oauth_test.go.

func TestNewWatcher_Defaults(t *testing.T) {
	p := NewPool(&stubDoer{}, PoolOptions{})
	w := NewWatcher(p, 0, nil)
	if w.interval != 5*time.Minute {
		t.Errorf("default interval: %v", w.interval)
	}
	if w.log == nil {
		t.Error("default log should be set")
	}
	if w.pool != p {
		t.Error("pool not bound")
	}
}

func TestNewWatcher_Custom(t *testing.T) {
	p := NewPool(&stubDoer{}, PoolOptions{})
	w := NewWatcher(p, 30*time.Second, nil)
	if w.interval != 30*time.Second {
		t.Errorf("interval: %v", w.interval)
	}
}

func TestWatcher_StartStop(t *testing.T) {
	p := NewPool(&stubDoer{}, PoolOptions{})
	w := NewWatcher(p, time.Hour, nil) // long interval so it doesn't actually tick
	stop := w.Start(context.Background())
	// Give the goroutine a moment to launch.
	time.Sleep(10 * time.Millisecond)
	stop() // should return promptly
	// Calling stop twice should be safe (idempotent).
	stop()
}

func TestWatcher_TickEmptyPool(t *testing.T) {
	p := NewPool(&stubDoer{}, PoolOptions{})
	w := NewWatcher(p, time.Hour, nil)
	// Should not panic with empty pool.
	w.tick(context.Background())
}

func TestWatcher_TickRefreshes(t *testing.T) {
	// Account with a token that's about to expire and a refresh_token.
	// The watcher calls RefreshAccessToken(ctx, nil, ...) — i.e. it builds a
	// real TLS client and hits auth.openai.com. We can't stub it without
	// changing the watcher signature. Instead, verify the calling convention
	// by injecting a token whose refresh_token is the empty string — the
	// watcher should skip it without making a network call.
	a := &Account{AccessToken: "old-tok", Status: StatusNormal, RefreshToken: ""}
	p := NewPool(&stubDoer{}, PoolOptions{})
	p.Upsert(a)
	w := NewWatcher(p, time.Hour, nil)
	w.tick(context.Background())
	// Account should still exist and not have been aliased.
	if p.ResolveToken("old-tok") != "old-tok" {
		t.Errorf("empty refresh token should not trigger alias, got %q", p.ResolveToken("old-tok"))
	}
}

func TestWatcher_TickSkipsExpiredAccountsWithoutRefreshToken(t *testing.T) {
	// No refresh token → tick should skip without erroring.
	a := &Account{AccessToken: "tok", Status: StatusNormal}
	p := NewPool(&stubDoer{}, PoolOptions{})
	p.Upsert(a)
	w := NewWatcher(p, time.Hour, nil)
	w.tick(context.Background())
	// Account should still be there.
	if p.Size() != 1 {
		t.Errorf("account should remain, got size=%d", p.Size())
	}
}

func TestWatcher_TickRestoresLimitedAfterRestoreAt(t *testing.T) {
	p := NewPool(&stubDoer{}, PoolOptions{})
	p.Upsert(&Account{
		AccessToken: "tok",
		Status:      StatusLimited,
		Quota:       0,
		RestoreAt:   time.Now().Add(-time.Minute), // already past
	})
	w := NewWatcher(p, time.Hour, nil)
	w.tick(context.Background())
	list := p.List()
	if len(list) != 1 || list[0].Status != StatusNormal {
		t.Errorf("expected StatusNormal, got %+v", list)
	}
	if list[0].Quota < 1 {
		t.Errorf("quota should be >=1, got %d", list[0].Quota)
	}
}

func TestWatcher_TickStopsOnContextCancel(t *testing.T) {
	p := NewPool(&stubDoer{}, PoolOptions{})
	w := NewWatcher(p, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Should not block on cancelled ctx.
	done := make(chan struct{})
	go func() {
		w.tick(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("tick did not return promptly on cancelled ctx")
	}
}

// Sanity: stubDoer implements httpclient.Doer.
var _ httpclient.Doer = (*stubDoer)(nil)