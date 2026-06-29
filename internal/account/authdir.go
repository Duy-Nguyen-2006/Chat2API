package account

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultAuthDir = "auth"

// DiscoverCookieFiles lists cookie export JSON files under dir.
// Matches cookies*.json and *.json; skips dotfiles and non-regular files.
func DiscoverCookieFiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("account: read auth dir %s: %w", dir, err)
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths, nil
}

// HydrateFromAuthDir loads every cookie file in dir into the pool.
// Existing entries keyed by the same cookies_file path are replaced first
// so refreshed exports do not leave stale tokens behind.
// When fallbackToken is set it is used for the first cookie file only
// (pairs CHATGPT_ACCESS_TOKEN from .env with auth/cookies_*.json).
func HydrateFromAuthDir(pool *Pool, dir, fallbackToken string) (int, error) {
	if pool == nil {
		return 0, nil
	}
	if dir == "" {
		dir = defaultAuthDir
	}
	files, err := DiscoverCookieFiles(dir)
	if err != nil {
		return 0, err
	}
	var loaded int
	var errs []string
	for i, path := range files {
		token := ""
		if i == 0 && fallbackToken != "" {
			token = fallbackToken
		}
		acc, err := MigrateFromCookiesWithToken(path, token)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(path), err))
			continue
		}
		acc.CookiesFile = path
		pool.RemoveByCookiesFile(path)
		pool.Upsert(acc)
		loaded++
	}
	if len(errs) > 0 && loaded == 0 {
		return 0, fmt.Errorf("account: auth dir %s: %s", dir, strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		fmt.Printf("[Server] auth dir warnings: %s\n", strings.Join(errs, "; "))
	}
	return loaded, nil
}

// CookiesPath returns the per-account cookies file, else fallback from config.
func CookiesPath(a *Account, fallback string) string {
	if a != nil && a.CookiesFile != "" {
		return a.CookiesFile
	}
	return fallback
}