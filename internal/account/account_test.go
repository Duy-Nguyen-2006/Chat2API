package account

import "testing"

func TestAccountAvailable(t *testing.T) {
	cases := []struct {
		name    string
		acc     *Account
		isImage bool
		want    bool
	}{
		{"nil", nil, false, false},
		{"normal-text", &Account{Status: StatusNormal}, false, true},
		{"normal-image-with-quota", &Account{Status: StatusNormal, Quota: 5}, true, true},
		{"normal-image-no-quota", &Account{Status: StatusNormal, Quota: 0}, true, false},
		{"normal-image-quota-unknown", &Account{Status: StatusNormal, Quota: 0, ImageQuotaUnknow: true}, true, true},
		{"disabled", &Account{Status: StatusDisabled}, false, false},
		{"disabled-image", &Account{Status: StatusDisabled, Quota: 5, ImageQuotaUnknow: true}, true, false},
		{"error", &Account{Status: StatusError}, false, false},
		{"limited-text", &Account{Status: StatusLimited}, false, true},
		{"limited-image-with-quota", &Account{Status: StatusLimited, Quota: 1}, true, true},
	}
	for _, c := range cases {
		if got := c.acc.Available(c.isImage); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAccountCensor(t *testing.T) {
	a := &Account{
		AccessToken:  "eyJabcdefghijk-1234567890",
		RefreshToken: "rt-secret-1234567890",
		Cookie:       "session=verysecret",
		Password:     "hunter2",
		AccountID:    "acc-1",
		Email:        "alice@example.com",
		Status:       StatusNormal,
	}
	got := a.Censor()
	if got.AccessToken != "eyJabc***" {
		t.Errorf("token redaction: %q", got.AccessToken)
	}
	if got.RefreshToken != "rt-sec***" {
		t.Errorf("refresh redaction: %q", got.RefreshToken)
	}
	if got.Cookie != "sessio***" {
		t.Errorf("cookie redaction: %q", got.Cookie)
	}
	if got.Password != "" {
		t.Errorf("password should be cleared: %q", got.Password)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email should be preserved: %q", got.Email)
	}
	if got.AccountID != "acc-1" {
		t.Errorf("account id should be preserved: %q", got.AccountID)
	}
	// Original must not be mutated.
	if a.AccessToken != "eyJabcdefghijk-1234567890" {
		t.Errorf("original mutated: %q", a.AccessToken)
	}
}

func TestAccountCensor_ShortStrings(t *testing.T) {
	a := &Account{AccessToken: "abc"}
	got := a.Censor()
	if got.AccessToken != "***" {
		t.Errorf("short token: %q", got.AccessToken)
	}
	a2 := &Account{AccessToken: ""}
	if got := a2.Censor(); got.AccessToken != "" {
		t.Errorf("empty: %q", got.AccessToken)
	}
}

func TestAccountCensor_Nil(t *testing.T) {
	var a *Account
	if got := a.Censor(); got != nil {
		t.Errorf("nil: %+v", got)
	}
}

func TestRedact(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"a", "***"},
		{"abcdef", "***"},   // exactly 6 → *** (no chars visible)
		{"abcdefg", "abcdef***"}, // 7 chars → 6 visible
		{"abcdefgh", "abcdef***"},
	}
	for _, c := range cases {
		if got := redact(c.in); got != c.want {
			t.Errorf("redact(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}