package protocol

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAnthropicMessages_BadJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader("not json"))
	HandleAnthropicMessages(rr, req, nil)
	if rr.Code != 400 {
		t.Errorf("code: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request body") {
		t.Errorf("body: %s", rr.Body.String())
	}
}

func TestHandleAnthropicMessages_NoMessages(t *testing.T) {
	body := `{"model":"claude-3-5-sonnet","messages":[]}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	HandleAnthropicMessages(rr, req, nil)
	if rr.Code != 400 {
		t.Errorf("code: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "messages required") {
		t.Errorf("body: %s", rr.Body.String())
	}
}

func TestWriteAnthropicEventShape(t *testing.T) {
	// Sanity-check the event format: event: <type>\ndata: <json>\n\n
	rr := httptest.NewRecorder()
	writeAnthropicEvent(rr, &nopFlusher{}, map[string]any{
		"type": "ping",
	})
	out := rr.Body.String()
	if !strings.HasPrefix(out, "event: ping\ndata: ") {
		t.Errorf("format wrong: %q", out)
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Errorf("should end with blank line: %q", out)
	}
}

type nopFlusher struct{}

func (nopFlusher) Flush() {}