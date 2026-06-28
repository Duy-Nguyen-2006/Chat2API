package chatgpt

import (
	"testing"
	"time"
)

func TestDecodeImagePayloadDataURI(t *testing.T) {
	// "hello" base64 = aGVsbG8= ; we'll wrap it in a data URI
	data, name, mime := DecodeImagePayload("data:image/png;base64,aGVsbG8=")
	if string(data) != "hello" {
		t.Errorf("data mismatch: %q", data)
	}
	if name != "image.png" {
		t.Errorf("name: %q", name)
	}
	if mime != "image/png" {
		t.Errorf("mime: %q", mime)
	}
}

func TestDecodeImagePayloadJPEG(t *testing.T) {
	data, name, mime := DecodeImagePayload("data:image/jpeg;base64,aGVsbG8=")
	if string(data) != "hello" {
		t.Errorf("data: %q", data)
	}
	if name != "image.jpg" {
		t.Errorf("jpeg should map to .jpg, got %q", name)
	}
	if mime != "image/jpeg" {
		t.Errorf("mime: %q", mime)
	}
}

func TestDecodeImagePayloadRawBase64(t *testing.T) {
	data, name, mime := DecodeImagePayload("aGVsbG8=")
	if string(data) != "hello" {
		t.Errorf("data: %q", data)
	}
	if name != "image.png" || mime != "image/png" {
		t.Errorf("defaults wrong: name=%q mime=%q", name, mime)
	}
}

func TestImageModelSlug(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"gpt-image-2", "gpt-5-3"},
		{"GPT-IMAGE-2", "gpt-5-3"},
		{"codex-gpt-image-2", "codex-gpt-image-2"},
		{"unknown", "auto"},
		{"", "auto"},
	}
	for _, c := range cases {
		if got := imageModelSlug(c.in); got != c.want {
			t.Errorf("imageModelSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(100*time.Millisecond, time.Second); got != 200*time.Millisecond {
		t.Errorf("double: %v", got)
	}
	if got := nextBackoff(time.Second, time.Second); got != time.Second {
		t.Errorf("cap: %v", got)
	}
	if got := nextBackoff(2*time.Second, time.Second); got != time.Second {
		t.Errorf("cap-overflow: %v", got)
	}
}

func TestDedup(t *testing.T) {
	got := dedup([]string{"a", "b", "a", "c", "b", ""})
	if len(got) != 4 {
		t.Errorf("len: %d (%v)", len(got), got)
	}
	want := map[string]bool{"a": true, "b": true, "c": true, "": true}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected: %q", g)
		}
	}
}

func TestJoinSorted(t *testing.T) {
	// joinSorted is documented as producing a stable joined key — order is
	// insertion order, not lexicographic. The caller is responsible for any
	// required sorting.
	got := joinSorted([]string{"c", "a", "b"})
	want := "c,a,b"
	if got != want {
		t.Errorf("joinSorted = %q, want %q", got, want)
	}
	if joinSorted(nil) != "" {
		t.Errorf("nil should be empty")
	}
}

func TestExtractImageArtifacts(t *testing.T) {
	conv := map[string]any{
		"mapping": map[string]any{
			"node-1": map[string]any{
				"message": map[string]any{
					"author": map[string]any{"role": "assistant"},
					"content": map[string]any{
						"parts": []any{
							map[string]any{"asset_pointer": "file-service://file-abc"},
						},
					},
					"metadata": map[string]any{
						"attachments": []any{
							map[string]any{"id": "sed-xyz"},
						},
					},
				},
			},
		},
	}
	files, seds := extractImageArtifacts(conv)
	if len(files) != 1 || files[0] != "file-abc" {
		t.Errorf("files: %v", files)
	}
	if len(seds) != 1 || seds[0] != "sed-xyz" {
		t.Errorf("sediments: %v", seds)
	}
}

func TestExtractImageArtifacts_IgnoresUserAuthor(t *testing.T) {
	conv := map[string]any{
		"mapping": map[string]any{
			"node-1": map[string]any{
				"message": map[string]any{
					"author": map[string]any{"role": "user"},
					"content": map[string]any{
						"parts": []any{
							map[string]any{"asset_pointer": "file-service://should-skip"},
						},
					},
				},
			},
		},
	}
	files, _ := extractImageArtifacts(conv)
	if len(files) != 0 {
		t.Errorf("user messages should be skipped, got %v", files)
	}
}

func TestIsTransientConversationErr(t *testing.T) {
	if !isTransientConversationErr(&transientHTTPError{status: 502}) {
		t.Error("transientHTTPError should be transient")
	}
	if isTransientConversationErr(nil) {
		t.Error("nil err should not be transient")
	}
}