package protocol

import (
	"strings"
	"testing"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
)

func TestIsImageModel(t *testing.T) {
	cases := map[string]bool{
		"gpt-image-2":       true,
		"GPT-IMAGE-2":       true,
		"codex-gpt-image-2": true,
		"gpt-5":             false,
		"auto":              false,
		"":                  false,
	}
	for in, want := range cases {
		if got := chatgpt.IsImageModel(in); got != want {
			t.Errorf("IsImageModel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDecodeImagePayloadDataURI(t *testing.T) {
	// "data:image/png;base64,aGVsbG8=" decodes to "hello".
	data, name, mime := chatgpt.DecodeImagePayload("data:image/png;base64,aGVsbG8=")
	if string(data) != "hello" {
		t.Errorf("payload mismatch: %q", data)
	}
	if name != "image.png" {
		t.Errorf("filename: %q", name)
	}
	if mime != "image/png" {
		t.Errorf("mime: %q", mime)
	}
}

func TestDecodeImagePayloadRaw(t *testing.T) {
	data, _, _ := chatgpt.DecodeImagePayload("aGVsbG8=")
	if string(data) != "hello" {
		t.Errorf("raw base64 mismatch: %q", data)
	}
}

func TestSSEFindEvent(t *testing.T) {
	buf := []byte("data: foo\n\ndata: bar\n\n")
	idx := sseFindEvent(buf)
	if idx < 0 {
		t.Fatal("expected to find event boundary")
	}
	// idx points at the 'd' of the next event (after the "\n\n" boundary).
	if !strings.HasPrefix(string(buf[idx:]), "data: bar") {
		t.Errorf("next event wrong: %q", buf[idx:])
	}
	// The current event is everything up to (but not including) the boundary.
	if got := string(buf[:idx-2]); got != "data: foo" {
		t.Errorf("event payload wrong: %q", got)
	}
}

func TestSSEStateExtractsConversationID(t *testing.T) {
	state := newSSEState()
	state.feed([]byte(`data: {"conversation_id":"abc-123","other":1}`))
	if state.conversationID != "abc-123" {
		t.Errorf("conversation_id: %q", state.conversationID)
	}
}

func TestSSEStateIgnoresDoneAndEmpty(t *testing.T) {
	state := newSSEState()
	state.feed([]byte("data: [DONE]"))
	state.feed([]byte("data: "))
	if state.conversationID != "" {
		t.Errorf("conversation_id should be empty, got %q", state.conversationID)
	}
}
