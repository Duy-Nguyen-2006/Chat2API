package chatgpt

import "testing"

func TestSolveTurnstileTokenInvalidInputReturnsEmpty(t *testing.T) {
	// Garbage dx must not panic and must return "".
	cases := []string{"", "!!!not-base64!!!", "aGVsbG8="}
	for _, dx := range cases {
		if got := solveTurnstileToken(dx, "key"); got != "" {
			t.Errorf("solveTurnstileToken(%q) = %q, want empty", dx, got)
		}
	}
}

func TestTurnstileStrHelpers(t *testing.T) {
	if got := turnstileStr(nil); got != "undefined" {
		t.Errorf("turnstileStr(nil) = %q, want undefined", got)
	}
	if got := turnstileStr("window.Math"); got != "[object Math]" {
		t.Errorf("turnstileStr(window.Math) = %q", got)
	}
	if got := turnstileStr(float64(42)); got != "42" {
		t.Errorf("turnstileStr(42) = %q, want 42", got)
	}
}

func TestXorStringRoundTrip(t *testing.T) {
	original := "hello turnstile"
	key := "k"
	xored := xorString(original, key)
	if xored == original {
		t.Fatal("xorString produced no change")
	}
	if back := xorString(xored, key); back != original {
		t.Errorf("xor round-trip failed: got %q want %q", back, original)
	}
}
