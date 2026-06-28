package chatgpt

import (
	"testing"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

func TestRequirementsTokenPrefix(t *testing.T) {
	ua := httpclient.NewFingerprint().UserAgent
	token := requirementsToken(ua)
	if len(token) < 10 {
		t.Fatalf("token too short: %q", token)
	}
	if token[:7] != "gAAAAAC" {
		t.Fatalf("unexpected prefix: %q", token[:7])
	}
}

func TestAnswerTokenSolves(t *testing.T) {
	ua := httpclient.NewFingerprint().UserAgent
	cfg := newPowConfig(ua)
	seed := "0.12345"
	diff := "0fffff"
	answer, solved := generatePowAnswer(seed, diff, cfg)
	if !solved {
		t.Fatal("expected PoW to solve for easy difficulty")
	}
	token, ok := answerToken(seed, diff, ua)
	if !ok || token[:7] != "gAAAAAB" {
		t.Fatalf("answer token invalid: ok=%v token=%q", ok, token)
	}
	_ = answer
}
