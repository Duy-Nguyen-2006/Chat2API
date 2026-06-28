package httpclient

import (
	"net/http"
	"testing"
)

func TestNewBuildsClient(t *testing.T) {
	c, err := New(DefaultOptions())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if c == nil {
		t.Fatal("New returned nil client")
	}
}

func TestDefaultProfileIsSet(t *testing.T) {
	// DefaultProfile is a profiles.ClientProfile (struct with an interface
	// field, hence not comparable with ==); just sanity-check the package-level
	// value is usable by building a client from it.
	if _, err := New(Options{Profile: DefaultProfile}); err != nil {
		t.Fatalf("New with DefaultProfile failed: %v", err)
	}
}

func TestNewWithProxy(t *testing.T) {
	// Proxy option must not error at construction time (validation is lazy).
	opts := DefaultOptions()
	opts.ProxyURL = "socks5://127.0.0.1:1080"
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New with proxy failed: %v", err)
	}
	if c == nil {
		t.Fatal("New returned nil client")
	}
}

func TestFingerprintApplySetsClientHints(t *testing.T) {
	fp := NewFingerprint()
	h := make(http.Header)
	fp.Apply(h)

	required := []string{
		"User-Agent", "Oai-Device-Id", "Oai-Session-Id",
		"OAI-Client-Version", "Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform",
	}
	for _, key := range required {
		if h.Get(key) == "" {
			t.Errorf("missing required client hint header: %s", key)
		}
	}
	if h.Get("Oai-Device-Id") == h.Get("Oai-Session-Id") {
		t.Error("device id and session id should differ")
	}
}
