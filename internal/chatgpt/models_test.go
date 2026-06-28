package chatgpt

import "testing"

func TestIsImageModel(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"gpt-image-2", true},
		{"codex-gpt-image-2", true},
		{"GPT-IMAGE-2", true},     // case-insensitive
		{"  gpt-image-2  ", true}, // trims whitespace
		{"gpt-4o", false},
		{"gpt-5", false},
		{"auto", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsImageModel(c.in); got != c.want {
			t.Errorf("IsImageModel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestImageModelIDs(t *testing.T) {
	ids := ImageModelIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 image models, got %d: %v", len(ids), ids)
	}
	want := map[string]bool{"gpt-image-2": true, "codex-gpt-image-2": true}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected image model id: %q", id)
		}
	}
}

func TestMapModel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"gpt-5", "gpt-5"},
		{"gpt-5-3-mini", "gpt-5-3-mini"},
		{"GPT-5-3", "gpt-5-3"},            // case-insensitive
		{"gpt-4o", "gpt-4o"},
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"o1", "o1"},
		{"o1-mini", "o1-mini"},
		{"o3-mini", "o3-mini"},
		{"o3-mini-high", "o3-mini-high"}, // more specific wins
		{"gpt-3.5-turbo", "text-davinci-002-render-sha"},
		{"auto", "auto"},
		{"g-unknown123", "auto"}, // gizmo id collapses to auto
		{"unknown-model", "gpt-4o"}, // default fallback
	}
	for _, c := range cases {
		if got := MapModel(c.in); got != c.want {
			t.Errorf("MapModel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSupportedModelsIncludes(t *testing.T) {
	want := []string{"gpt-5", "gpt-4o", "auto", "gpt-image-2", "o3-mini-high"}
	have := make(map[string]bool, len(SupportedModels))
	for _, m := range SupportedModels {
		have[m] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("SupportedModels missing %q", w)
		}
	}
}