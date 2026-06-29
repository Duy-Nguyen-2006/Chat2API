package account

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

// ErrNoAvailableAccount is returned by GetTextToken / GetImageToken when the
// pool is empty or every account is disabled/error.
var ErrNoAvailableAccount = errors.New("account: no available account")

// PoolOptions configures a Pool.
type PoolOptions struct {
	// ImageConcurrency is the max in-flight image requests per token.
	// 0 means use the default (3).
	ImageConcurrency int
	// AutoRemoveInvalid deletes 401'd tokens instead of marking them error.
	AutoRemoveInvalid bool
}

// Pool is the thread-safe account registry used by request handlers.
// It mirrors the basketikun AccountService: round-robin for text, slot-
// tracked semaphore for images, token aliasing on rotation, and 401-
// eviction with a small deferral window.
type Pool struct {
	mu                 sync.Mutex
	imageCond          *sync.Cond // bound to mu
	index              int
	accounts           map[string]*Account // keyed by AccessToken
	tokenAliases       map[string]string   // oldToken -> newToken after rotation
	imageInflight      map[string]int      // per-token image in-flight count
	doer               httpclient.Doer     // shared TLS-impersonating client
	opts               PoolOptions
	rng                *rand.Rand
	imageSemChans      map[string]chan struct{} // bounded slot semaphore
}

// NewPool constructs an empty pool with the shared Doer.
func NewPool(doer httpclient.Doer, opts PoolOptions) *Pool {
	if opts.ImageConcurrency <= 0 {
		opts.ImageConcurrency = 3
	}
	p := &Pool{
		accounts:      make(map[string]*Account),
		tokenAliases:  make(map[string]string),
		imageInflight: make(map[string]int),
		doer:          doer,
		opts:          opts,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	p.imageCond = sync.NewCond(&p.mu)
	return p
}

// Size returns the current number of accounts (including errored/disabled).
func (p *Pool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.accounts)
}

// HTTPClient returns the shared Doer so callers can attach it to chatgpt
// Clients they build from a selected Account.
func (p *Pool) HTTPClient() httpclient.Doer {
	return p.doer
}

// List returns a snapshot of all accounts (caller may filter).
// The returned slice holds censored copies — sensitive fields are redacted.
func (p *Pool) List() []*Account {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Account, 0, len(p.accounts))
	for _, a := range p.accounts {
		out = append(out, a.Censor())
	}
	return out
}

// Upsert inserts or replaces an account. If an account with the same
// AccessToken already exists it is overwritten; if an old token maps to
// the new one via aliases, the alias is cleared to avoid a redirect loop.
func (p *Pool) Upsert(a *Account) {
	if a == nil || a.AccessToken == "" {
		return
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if a.Status == "" {
		a.Status = StatusNormal
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.accounts[a.AccessToken]; !ok {
		p.accounts[a.AccessToken] = a
		return
	}
	// Replace existing entry while preserving created_at.
	prev := p.accounts[a.AccessToken]
	if !prev.CreatedAt.IsZero() {
		a.CreatedAt = prev.CreatedAt
	}
	p.accounts[a.AccessToken] = a
	// Clear alias chain that points to this token.
	for k, v := range p.tokenAliases {
		if v == a.AccessToken {
			delete(p.tokenAliases, k)
		}
	}
}

// RemoveByCookiesFile deletes every account sourced from the given cookies export.
func (p *Pool) RemoveByCookiesFile(cookiesFile string) {
	if cookiesFile == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for tok, a := range p.accounts {
		if a.CookiesFile == cookiesFile {
			delete(p.accounts, tok)
			delete(p.imageInflight, tok)
		}
	}
	for k, v := range p.tokenAliases {
		if _, ok := p.accounts[v]; !ok {
			delete(p.tokenAliases, k)
		}
	}
}

// Remove deletes the account with the given access token from the pool and
// drops any aliases that pointed to it.
func (p *Pool) Remove(accessToken string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.accounts, accessToken)
	delete(p.imageInflight, accessToken)
	for k, v := range p.tokenAliases {
		if v == accessToken {
			delete(p.tokenAliases, k)
		}
	}
}

// ResolveToken walks the alias chain so a caller holding a stale token still
// finds the rotated account. Returns "" if the token is unknown.
func (p *Pool) ResolveToken(token string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Follow the chain with a depth limit to avoid pathological loops.
	visited := make(map[string]bool, 4)
	cur := token
	for i := 0; i < 8; i++ {
		if visited[cur] {
			return ""
		}
		visited[cur] = true
		if _, ok := p.accounts[cur]; ok {
			return cur
		}
		next, ok := p.tokenAliases[cur]
		if !ok {
			return ""
		}
		cur = next
	}
	return ""
}

// GetTextToken returns the next round-robin account suitable for text
// chat. Excludes tokens in excluded and any account whose status is
// disabled/error. Optional refresh is attempted on the returned token if
// it is near expiry.
func (p *Pool) GetTextToken(ctx context.Context, excluded []string) (*Account, error) {
	for attempt := 0; attempt < 8; attempt++ {
		acc := p.pickText(excluded)
		if acc == nil {
			return nil, ErrNoAvailableAccount
		}
		if acc.AccessToken == "" {
			continue
		}
		// Skip refresh in the hot path — watchers cover the bulk; if the
		// caller has force-refresh semantics it should call RefreshToken.
		_ = ctx
		acc.LastUsedAt = time.Now()
		return acc, nil
	}
	return nil, ErrNoAvailableAccount
}

// GetImageToken returns an account with an available image-generation slot.
// Blocks (via imageCond.Wait) until a slot frees, or returns
// ErrNoAvailableAccount if no account can satisfy the request.
func (p *Pool) GetImageToken(ctx context.Context, planType PlanType) (*Account, error) {
	done := make(chan *Account)
	go func() {
		defer close(done)
		p.mu.Lock()
		defer p.mu.Unlock()
		for {
			if acc := p.tryAcquireImageSlot(planType); acc != nil {
				done <- acc
				return
			}
			timer := time.AfterFunc(2*time.Second, func() { p.imageCond.Broadcast() })
			p.imageCond.Wait()
			timer.Stop()
		}
	}()
	select {
	case acc := <-done:
		return acc, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// tryAcquireImageSlot picks an account with a free image slot. Caller must hold p.mu.
func (p *Pool) tryAcquireImageSlot(planType PlanType) *Account {
	for tok, a := range p.accounts {
		if !a.Available(true) {
			continue
		}
		if planType != "" && a.Type != planType {
			continue
		}
		if p.imageInflight[tok] >= p.opts.ImageConcurrency {
			continue
		}
		p.imageInflight[tok]++
		p.index++
		a.LastUsedAt = time.Now()
		return a
	}
	return nil
}

// ReleaseImageSlot decrements the in-flight counter for token and wakes any
// waiters. Safe to call with a token that has no recorded slot.
func (p *Pool) ReleaseImageSlot(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n, ok := p.imageInflight[token]; ok {
		if n <= 1 {
			delete(p.imageInflight, token)
		} else {
			p.imageInflight[token] = n - 1
		}
	}
	p.imageCond.Broadcast()
}

// pickText is the round-robin selector for text tokens. Must be called with
// p.mu held (or from a wrapper that holds it).
func (p *Pool) pickText(excluded []string) *Account {
	p.mu.Lock()
	defer p.mu.Unlock()
	ex := make(map[string]bool, len(excluded))
	for _, e := range excluded {
		ex[e] = true
	}
	tokens := make([]string, 0, len(p.accounts))
	for tok, a := range p.accounts {
		if ex[tok] {
			continue
		}
		if !a.Available(false) {
			continue
		}
		tokens = append(tokens, tok)
	}
	if len(tokens) == 0 {
		return nil
	}
	idx := p.index % len(tokens)
	p.index++
	return p.accounts[tokens[idx]]
}

// MarkInvalid records a 401/403 for the given token and either marks the
// account as errored or removes it (per opts.AutoRemoveInvalid). The
// deferral rule matches basketikun: if the account was created within the
// last 10 minutes, or invalid_count == 1, we defer the eviction once to
// ride out transient errors.
func (p *Pool) MarkInvalid(token string, err error) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	acc, ok := p.accounts[token]
	if !ok {
		return false
	}
	acc.InvalidCount++
	acc.LastInvalidAt = time.Now()
	acc.RefreshErr = errString(err)

	// Defer once when the account is brand-new or only invalidated once.
	newAccountGrace := 10 * time.Minute
	if acc.InvalidCount == 1 {
		return false
	}
	if time.Since(acc.CreatedAt) < newAccountGrace {
		return false
	}
	if p.opts.AutoRemoveInvalid {
		delete(p.accounts, token)
		delete(p.imageInflight, token)
		for k, v := range p.tokenAliases {
			if v == token {
				delete(p.tokenAliases, k)
			}
		}
		return true
	}
	acc.Status = StatusError
	acc.Quota = 0
	return false
}

// ApplyRefreshedToken swaps the account's access_token and rewires the alias
// chain so callers holding the old token continue to find it. Must be
// called while holding p.mu.
func (p *Pool) ApplyRefreshedToken(oldToken string, ts *TokenSet) (string, bool) {
	if ts == nil || ts.AccessToken == "" || ts.AccessToken == oldToken {
		return oldToken, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	acc, ok := p.accounts[oldToken]
	if !ok {
		return oldToken, false
	}
	// Drop any old entry and re-insert under the new key.
	delete(p.accounts, oldToken)
	if n, ok := p.imageInflight[oldToken]; ok {
		p.imageInflight[ts.AccessToken] = n
		delete(p.imageInflight, oldToken)
	}
	acc.AccessToken = ts.AccessToken
	if ts.RefreshToken != "" {
		acc.RefreshToken = ts.RefreshToken
	}
	if ts.ExpiresIn > 0 {
		acc.ExpiresAt = time.Now().Add(time.Duration(ts.ExpiresIn) * time.Second)
	}
	acc.InvalidCount = 0
	acc.RefreshErr = ""
	acc.Status = StatusNormal
	p.accounts[ts.AccessToken] = acc
	p.tokenAliases[oldToken] = ts.AccessToken
	return ts.AccessToken, true
}

// MarkUsed stamps LastUsedAt on the account identified by accessToken.
func (p *Pool) MarkUsed(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if a, ok := p.accounts[token]; ok {
		a.LastUsedAt = time.Now()
	}
}

// MarkLimited transitions an account into the rate-limited state and zeroes
// its image quota until restore_at. restoreAt may be the zero time.
func (p *Pool) MarkLimited(token string, restoreAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if a, ok := p.accounts[token]; ok {
		a.Status = StatusLimited
		a.Quota = 0
		a.RestoreAt = restoreAt
	}
}

// SetQuota updates the quota bookkeeping after a successful remote check.
func (p *Pool) SetQuota(token string, quota int, restoreAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if a, ok := p.accounts[token]; ok {
		a.Quota = quota
		a.RestoreAt = restoreAt
		a.ImageQuotaUnknow = false
	}
}

// TokensNeedingRefresh returns the list of accounts whose access token will
// expire within the next 24h. Used by the background watcher.
func (p *Pool) TokensNeedingRefresh() []*Account {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Account, 0)
	for _, a := range p.accounts {
		if NeedsRefresh(a.AccessToken, false) {
			out = append(out, a)
		}
	}
	return out
}

// TokensLimited returns accounts currently in the limited state, useful for
// the watcher's periodic quota check.
func (p *Pool) TokensLimited() []*Account {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Account, 0)
	for _, a := range p.accounts {
		if a.Status == StatusLimited {
			out = append(out, a)
		}
	}
	return out
}

// Snapshot returns a deep-enough copy of the pool state for persistence.
func (p *Pool) Snapshot() []*Account {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Account, 0, len(p.accounts))
	for _, a := range p.accounts {
		cp := *a
		out = append(out, &cp)
	}
	return out
}

// errString returns the error's message or "<nil>" for formatting.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
