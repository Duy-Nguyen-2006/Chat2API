package account

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Watcher periodically refreshes tokens nearing expiry and re-checks limited
// accounts. Run via Start(); cancel ctx to stop. Mirrors basketikun's
// start_limited_account_watcher but expressed as a self-contained Go loop.
type Watcher struct {
	pool     *Pool
	interval time.Duration
	log      *slog.Logger
}

// NewWatcher creates a Watcher that fires every interval.
func NewWatcher(p *Pool, interval time.Duration, log *slog.Logger) *Watcher {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{pool: p, interval: interval, log: log}
}

// Start launches the watcher goroutine. Returns a stop function the caller
// can defer to halt the loop on shutdown.
func (w *Watcher) Start(ctx context.Context) (stop func()) {
	cancelCtx, cancel := context.WithCancel(ctx)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(w.interval)
		defer t.Stop()
		// Fire once immediately, then on each tick.
		w.tick(cancelCtx)
		for {
			select {
			case <-cancelCtx.Done():
				return
			case <-t.C:
				w.tick(cancelCtx)
			}
		}
	}()
	return func() {
		cancel()
		wg.Wait()
	}
}

// tick runs one refresh + status-check cycle.
func (w *Watcher) tick(ctx context.Context) {
	// 1. Refresh tokens nearing expiry.
	for _, a := range w.pool.TokensNeedingRefresh() {
		if ctx.Err() != nil {
			return
		}
		if a.RefreshToken == "" {
			// No refresh_token — cannot rotate. Will likely hit a 401 on use.
			continue
		}
		ts, err := RefreshAccessToken(ctx, nil, a.RefreshToken)
		if err != nil {
			w.log.Warn("account: refresh failed", "email", DisplayName(a), "err", err)
			continue
		}
		newTok, rotated := w.pool.ApplyRefreshedToken(a.AccessToken, ts)
		if rotated {
			w.log.Info("account: refreshed", "old_token_prefix", a.AccessToken[:6]+"***", "new_token_prefix", newTok[:6]+"***")
		}
	}
	// 2. Re-evaluate limited tokens — the upstream may have lifted the cap.
	// A full quota probe belongs here once we have one; for now we just clear
	// "limited" state once restore_at has passed.
	for _, a := range w.pool.TokensLimited() {
		if !a.RestoreAt.IsZero() && time.Now().After(a.RestoreAt) {
			w.pool.SetQuota(a.AccessToken, 1, time.Time{}) // assume >=1 image slot restored
			a.Status = StatusNormal
			w.log.Info("account: limit restored", "email", DisplayName(a))
		}
	}
}
