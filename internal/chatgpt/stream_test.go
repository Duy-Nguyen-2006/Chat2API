package chatgpt

import "testing"

func TestExtractDelta(t *testing.T) {
	line := `data: {"message":{"author":{"role":"assistant"},"content":{"parts":["Hello"]}}}`
	if got := ExtractDelta(line); got != "Hello" {
		t.Fatalf("got %q, want Hello", got)
	}

	userLine := `data: {"message":{"author":{"role":"user"},"content":{"parts":["Hi"]}}}`
	if got := ExtractDelta(userLine); got != "" {
		t.Fatalf("user message should be skipped, got %q", got)
	}

	sw := NewStreamWriter("gpt-4o")
	if chunk := sw.processLine(line); chunk != "Hello" {
		t.Fatalf("first chunk: got %q", chunk)
	}
	line2 := `data: {"message":{"author":{"role":"assistant"},"content":{"parts":["Hello world"]}}}`
	if chunk := sw.processLine(line2); chunk != " world" {
		t.Fatalf("delta chunk: got %q, want ' world'", chunk)
	}
}