package chatgpt

import "testing"

func TestStreamMeta_ingestLine(t *testing.T) {
	var m StreamMeta
	m.ingestLine(`data: {"conversation_id":"conv-9","message":{"id":"msg-a","author":{"role":"assistant"},"content":{"parts":["hi"]}}}`)
	if m.ConversationID != "conv-9" {
		t.Fatalf("conversation_id: %q", m.ConversationID)
	}
	if m.ParentMessageID != "msg-a" {
		t.Fatalf("parent_message_id: %q", m.ParentMessageID)
	}
}