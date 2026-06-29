package chatgpt

import "testing"

func TestStreamChunkFromLine_LegacyFormat(t *testing.T) {
	line := `data: {"message":{"author":{"role":"assistant"},"content":{"parts":["Hello"]}}}`
	var acc string
	if chunk := streamChunkFromLine(line, &acc); chunk != "Hello" || acc != "Hello" {
		t.Fatalf("got chunk=%q acc=%q", chunk, acc)
	}
	line2 := `data: {"message":{"author":{"role":"assistant"},"content":{"parts":["Hello world"]}}}`
	if chunk := streamChunkFromLine(line2, &acc); chunk != " world" || acc != "Hello world" {
		t.Fatalf("delta chunk=%q acc=%q", chunk, acc)
	}
}

func TestStreamChunkFromLine_PatchEnvelope(t *testing.T) {
	line := `data: {"p":"","o":"add","v":{"message":{"author":{"role":"assistant"},"content":{"parts":["Hi"]},"id":"m1"},"conversation_id":"cid-1"},"c":0}`
	var acc string
	if chunk := streamChunkFromLine(line, &acc); chunk != "Hi" || acc != "Hi" {
		t.Fatalf("got chunk=%q acc=%q", chunk, acc)
	}
	meta := &StreamMeta{}
	meta.ingestRaw(map[string]any{"v": map[string]any{"conversation_id": "cid-1"}})
	if meta.ConversationID != "cid-1" {
		t.Fatalf("conversation id: %q", meta.ConversationID)
	}
}

func TestStreamChunkFromLine_PatchAppend(t *testing.T) {
	line := `data: {"o":"patch","v":[{"p":"/message/content/parts/0","o":"append","v":"Hey"}]}`
	var acc string
	if chunk := streamChunkFromLine(line, &acc); chunk != "Hey" || acc != "Hey" {
		t.Fatalf("got chunk=%q acc=%q", chunk, acc)
	}
	line2 := `data: {"o":"patch","v":[{"p":"/message/content/parts/0","o":"append","v":" there"}]}`
	if chunk := streamChunkFromLine(line2, &acc); chunk != " there" || acc != "Hey there" {
		t.Fatalf("append chunk=%q acc=%q", chunk, acc)
	}
}

func TestStreamChunkFromLine_StandaloneAppend(t *testing.T) {
	line := `data: {"p":"/message/content/parts/0","o":"append","v":"Hi!"}`
	var acc string
	if chunk := streamChunkFromLine(line, &acc); chunk != "Hi!" || acc != "Hi!" {
		t.Fatalf("got chunk=%q acc=%q", chunk, acc)
	}
}

func TestStreamChunkFromLine_SkipsUser(t *testing.T) {
	line := `data: {"v":{"message":{"author":{"role":"user"},"content":{"parts":["secret"]}}}}`
	var acc string
	if chunk := streamChunkFromLine(line, &acc); chunk != "" || acc != "" {
		t.Fatalf("should skip user message, got chunk=%q acc=%q", chunk, acc)
	}
}