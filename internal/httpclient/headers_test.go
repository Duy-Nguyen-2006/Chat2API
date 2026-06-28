package httpclient

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewFingerprint_Defaults(t *testing.T) {
	f := NewFingerprint()
	if f.UserAgent == "" || !strings.Contains(f.UserAgent, "Chrome/133") {
		t.Errorf("UserAgent: %q", f.UserAgent)
	}
	if f.DeviceID == "" || f.SessionID == "" {
		t.Errorf("DeviceID/SessionID should be set: %q / %q", f.DeviceID, f.SessionID)
	}
	if f.ClientVersion != DefaultClientVersion {
		t.Errorf("ClientVersion: %q", f.ClientVersion)
	}
	if f.ClientBuild != DefaultClientBuild {
		t.Errorf("ClientBuild: %q", f.ClientBuild)
	}
	if f.DeviceID == f.SessionID {
		t.Error("DeviceID and SessionID should differ")
	}
}

func TestNewFingerprint_UniqueIDs(t *testing.T) {
	a := NewFingerprint()
	b := NewFingerprint()
	if a.DeviceID == b.DeviceID {
		t.Error("DeviceID should be unique across calls")
	}
	if a.SessionID == b.SessionID {
		t.Error("SessionID should be unique across calls")
	}
}

func TestFingerprint_Apply(t *testing.T) {
	f := NewFingerprint()
	h := http.Header{}
	f.Apply(h)

	want := []string{
		"User-Agent", "Accept-Language", "Origin", "Referer",
		"Oai-Device-Id", "Oai-Session-Id",
		"OAI-Client-Version", "OAI-Client-Build-Number",
		"Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform",
		"Sec-Ch-Ua-Arch", "Sec-Ch-Ua-Bitness",
		"Sec-Ch-Ua-Model", "Sec-Ch-Ua-Platform-Version",
		"Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site", "Priority",
	}
	for _, k := range want {
		if h.Get(k) == "" {
			t.Errorf("missing header %s", k)
		}
	}
}

func TestFingerprint_Apply_EmptyFieldsSkip(t *testing.T) {
	f := BrowserFingerprint{} // everything empty
	h := http.Header{}
	f.Apply(h)
	// Static headers should still be set.
	if h.Get("Accept-Language") == "" {
		t.Error("Accept-Language should always be set")
	}
	if h.Get("Origin") == "" {
		t.Error("Origin should always be set")
	}
	// Optional per-account headers should be skipped when empty.
	if h.Get("User-Agent") != "" {
		t.Error("empty UserAgent should not set header")
	}
	if h.Get("Oai-Device-Id") != "" {
		t.Error("empty DeviceID should not set header")
	}
}

func TestNewUUID_Format(t *testing.T) {
	u := newUUID()
	parts := strings.Split(u, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 segments, got %d: %q", len(parts), u)
	}
	wantLens := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != wantLens[i] {
			t.Errorf("segment %d: got len %d, want %d (%q)", i, len(p), wantLens[i], p)
		}
	}
}