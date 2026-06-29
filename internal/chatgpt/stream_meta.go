package chatgpt

// StreamMeta captures upstream conversation threading ids from SSE.
type StreamMeta struct {
	ConversationID  string
	ParentMessageID string
	sawStreamData   bool
}

func (m *StreamMeta) ingestLine(line string) {
	raw, ok := parseSSEPayload(line)
	if !ok {
		return
	}
	m.ingestRaw(raw)
}

func (m *StreamMeta) ingestRaw(raw map[string]any) {
	m.sawStreamData = true
	if cid := conversationIDFromRaw(raw); cid != "" {
		m.ConversationID = cid
	}
	msg := streamMessageFromRaw(raw)
	if msg == nil {
		return
	}
	id, _ := msg["id"].(string)
	if id == "" {
		return
	}
	author, _ := msg["author"].(map[string]any)
	role, _ := author["role"].(string)
	switch role {
	case "assistant", "tool":
		m.ParentMessageID = id
	}
}