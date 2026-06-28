package config

import (
	"os"
	"testing"
)

// withEnv sets env vars for the duration of the test and restores them via t.Cleanup.
func withEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("CHATGPT_ACCESS_TOKEN", "")
	t.Setenv("CHATGPT_ACCOUNT_ID", "")
	t.Setenv("COOKIES_FILE", "")
	t.Setenv("ACCOUNTS_FILE", "")
	t.Setenv("REFRESH_ACCOUNT_INTERVAL_MIN", "")
	t.Setenv("IMAGE_ACCOUNT_CONCURRENCY", "")
	t.Setenv("AUTO_REMOVE_INVALID_ACCOUNTS", "")
	t.Setenv("AUTO_RELOGIN_AFTER_REFRESH", "")
	t.Setenv("PROXY", "")
	t.Setenv("AUTH_KEY", "")
	t.Setenv("DISABLE_AUTH", "")
	t.Setenv("STORAGE_BACKEND", "")
	t.Setenv("STORAGE_DIR", "")
	t.Setenv("SQLITE_PATH", "")
	t.Setenv("AUTO_APPROVE_TOOLS", "")

	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	withEnv(t, nil)
	c := Load()

	if c.Host != "localhost" {
		t.Errorf("Host: want localhost, got %q", c.Host)
	}
	if c.Port != 8080 {
		t.Errorf("Port: want 8080, got %d", c.Port)
	}
	if c.CookiesFile != "cookies_1.json" {
		t.Errorf("CookiesFile default: got %q", c.CookiesFile)
	}
	if c.AccountsFile != "accounts.json" {
		t.Errorf("AccountsFile default: got %q", c.AccountsFile)
	}
	if c.RefreshIntervalMin != 5 {
		t.Errorf("RefreshIntervalMin default: got %d", c.RefreshIntervalMin)
	}
	if c.ImageConcurrency != 3 {
		t.Errorf("ImageConcurrency default: got %d", c.ImageConcurrency)
	}
	if c.StorageType != "json" {
		t.Errorf("StorageType default: got %q", c.StorageType)
	}
	if c.StorageDir != "data" {
		t.Errorf("StorageDir default: got %q", c.StorageDir)
	}
	if !c.AutoApproveTools {
		t.Error("AutoApproveTools default should be true")
	}
	if c.AutoRemoveInvalid {
		t.Error("AutoRemoveInvalid default should be false")
	}
	if c.DisableAuth {
		t.Error("DisableAuth default should be false")
	}
}

func TestLoadFromEnv(t *testing.T) {
	withEnv(t, map[string]string{
		"HOST":                         "0.0.0.0",
		"PORT":                         "9090",
		"CHATGPT_ACCESS_TOKEN":         "tok-abc",
		"CHATGPT_ACCOUNT_ID":           "acc-xyz",
		"COOKIES_FILE":                 "custom.json",
		"ACCOUNTS_FILE":                "accs.json",
		"REFRESH_ACCOUNT_INTERVAL_MIN": "10",
		"IMAGE_ACCOUNT_CONCURRENCY":    "5",
		"AUTO_REMOVE_INVALID_ACCOUNTS": "true",
		"AUTO_RELOGIN_AFTER_REFRESH":   "1",
		"PROXY":                        "http://127.0.0.1:8888",
		"AUTH_KEY":                     "secret",
		"DISABLE_AUTH":                 "yes",
		"STORAGE_BACKEND":              "SQLITE",
		"STORAGE_DIR":                  "var/data",
		"SQLITE_PATH":                  "var/db.sqlite",
		"AUTO_APPROVE_TOOLS":           "false",
	})
	c := Load()

	if c.Host != "0.0.0.0" {
		t.Errorf("Host: got %q", c.Host)
	}
	if c.Port != 9090 {
		t.Errorf("Port: got %d", c.Port)
	}
	if c.ChatGPTToken != "tok-abc" {
		t.Errorf("ChatGPTToken: got %q", c.ChatGPTToken)
	}
	if c.ChatGPTAccountID != "acc-xyz" {
		t.Errorf("ChatGPTAccountID: got %q", c.ChatGPTAccountID)
	}
	if c.CookiesFile != "custom.json" {
		t.Errorf("CookiesFile: got %q", c.CookiesFile)
	}
	if c.AccountsFile != "accs.json" {
		t.Errorf("AccountsFile: got %q", c.AccountsFile)
	}
	if c.RefreshIntervalMin != 10 {
		t.Errorf("RefreshIntervalMin: got %d", c.RefreshIntervalMin)
	}
	if c.ImageConcurrency != 5 {
		t.Errorf("ImageConcurrency: got %d", c.ImageConcurrency)
	}
	if !c.AutoRemoveInvalid {
		t.Error("AutoRemoveInvalid should be true")
	}
	if !c.AutoRelogin {
		t.Error("AutoRelogin should be true")
	}
	if c.Proxy != "http://127.0.0.1:8888" {
		t.Errorf("Proxy: got %q", c.Proxy)
	}
	if c.AuthKey != "secret" {
		t.Errorf("AuthKey: got %q", c.AuthKey)
	}
	if !c.DisableAuth {
		t.Error("DisableAuth should be true")
	}
	if c.StorageType != "sqlite" {
		t.Errorf("StorageType should lowercase: got %q", c.StorageType)
	}
	if c.StorageDir != "var/data" {
		t.Errorf("StorageDir: got %q", c.StorageDir)
	}
	if c.SQLitePath != "var/db.sqlite" {
		t.Errorf("SQLitePath: got %q", c.SQLitePath)
	}
	if c.AutoApproveTools {
		t.Error("AutoApproveTools should be false")
	}
}

func TestLoadHostNormalisation(t *testing.T) {
	withEnv(t, map[string]string{"HOST": "127.0.0.1"})
	c := Load()
	if c.Host != "localhost" {
		t.Errorf("127.0.0.1 should normalise to localhost, got %q", c.Host)
	}
}

func TestLoadInvalidPortFallsBack(t *testing.T) {
	withEnv(t, map[string]string{"PORT": "not-a-number"})
	c := Load()
	if c.Port != 8080 {
		t.Errorf("invalid PORT should fall back to 8080, got %d", c.Port)
	}
}

func TestLoadInvalidRefreshIntervalFallsBack(t *testing.T) {
	withEnv(t, map[string]string{"REFRESH_ACCOUNT_INTERVAL_MIN": "x"})
	c := Load()
	if c.RefreshIntervalMin != 5 {
		t.Errorf("invalid interval should fall back to 5, got %d", c.RefreshIntervalMin)
	}
}

func TestEnvOr(t *testing.T) {
	if got := envOr("PATH", "fallback"); got != os.Getenv("PATH") {
		t.Errorf("envOr with PATH set: got %q", got)
	}
	os.Unsetenv("CHAT2API_TEST_UNSET_KEY")
	if got := envOr("CHAT2API_TEST_UNSET_KEY", "fb"); got != "fb" {
		t.Errorf("envOr unset: got %q", got)
	}
}

func TestEnvInt(t *testing.T) {
	os.Unsetenv("CHAT2API_TEST_INT")
	if got := envInt("CHAT2API_TEST_INT", 42); got != 42 {
		t.Errorf("envInt unset fallback: got %d", got)
	}
	t.Setenv("CHAT2API_TEST_INT", "123")
	if got := envInt("CHAT2API_TEST_INT", 42); got != 123 {
		t.Errorf("envInt set: got %d", got)
	}
	t.Setenv("CHAT2API_TEST_INT", "abc")
	if got := envInt("CHAT2API_TEST_INT", 42); got != 42 {
		t.Errorf("envInt invalid: got %d", got)
	}
}

func TestEnvBool(t *testing.T) {
	os.Unsetenv("CHAT2API_TEST_BOOL")
	if envBool("CHAT2API_TEST_BOOL") {
		t.Error("unset should be false")
	}
	for _, val := range []string{"1", "true", "TRUE", "Yes", "yes"} {
		t.Setenv("CHAT2API_TEST_BOOL", val)
		if !envBool("CHAT2API_TEST_BOOL") {
			t.Errorf("envBool(%q) should be true", val)
		}
	}
	for _, val := range []string{"", "0", "false", "no", "maybe"} {
		t.Setenv("CHAT2API_TEST_BOOL", val)
		if envBool("CHAT2API_TEST_BOOL") {
			t.Errorf("envBool(%q) should be false", val)
		}
	}
}