package chatgpt

import "testing"

func TestChatRequest_effectiveMessages(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "second"},
	}
	req := ChatRequest{ConversationID: "conv-1", Messages: msgs}
	got := req.effectiveMessages()
	if len(got) != 1 || got[0].Content != "second" {
		t.Fatalf("got %#v, want latest user only", got)
	}
	fresh := ChatRequest{Messages: msgs}
	if len(fresh.effectiveMessages()) != 3 {
		t.Fatalf("fresh conversation should keep all messages")
	}
}

func TestChatRequest_saveChatHistoryEnabled(t *testing.T) {
	falseVal := false
	req := ChatRequest{SaveChatHistory: &falseVal}
	if req.saveChatHistoryEnabled() {
		t.Fatal("expected disabled")
	}
	if !(ChatRequest{}).saveChatHistoryEnabled() {
		t.Fatal("default should enable history")
	}
}