package chatgpt

import "testing"

func TestExtractGizmoID(t *testing.T) {
	cases := map[string]string{
		"g-6a407590545c8191af136331cdcc4844":        "g-6a407590545c8191af136331cdcc4844",
		"g-p-6a3f94c1c0048191b54cd4eaa66de149":      "g-p-6a3f94c1c0048191b54cd4eaa66de149",
		"gpt-4o": "",
		"auto":   "",
	}
	for in, want := range cases {
		if got := extractGizmoID(in); got != want {
			t.Fatalf("extractGizmoID(%q) = %q, want %q", in, got, want)
		}
	}
}