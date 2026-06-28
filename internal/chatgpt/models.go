package chatgpt

import "strings"

// SupportedModels lists the model ids exposed via /v1/models. Includes
// text chat models and image-capable aliases (gpt-image-2, codex-gpt-image-2).
// Order is significant: MapModel walks it via string-search precedence, so
// the more-specific aliases (o3-mini-high) must precede the less-specific
// (o3-mini, o1, ...).
var SupportedModels = []string{
	// Image generation.
	"gpt-image-2",
	"codex-gpt-image-2",

	// GPT-5 family (newest).
	"gpt-5",
	"gpt-5-1",
	"gpt-5-2",
	"gpt-5-3",
	"gpt-5-3-mini",
	"gpt-5-mini",

	// GPT-4o family.
	"gpt-4o",
	"gpt-4o-mini",
	"gpt-4",
	"gpt-3.5-turbo",

	// Reasoning.
	"o1",
	"o1-pro",
	"o1-mini",
	"o1-preview",
	"o3-mini",
	"o3-mini-high",

	// Search / catch-all.
	"auto",
}

// ImageModelIDs returns the subset of SupportedModels usable with the image
// generation pipeline.
func ImageModelIDs() []string {
	out := make([]string, 0, 2)
	for _, m := range SupportedModels {
		switch m {
		case "gpt-image-2", "codex-gpt-image-2":
			out = append(out, m)
		}
	}
	return out
}

// IsImageModel returns true when the model id is recognised as an image
// generation model. Used by the image handler to route /v1/images/* vs
// /v1/chat/completions paths.
func IsImageModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	return lower == "gpt-image-2" || lower == "codex-gpt-image-2"
}

// MapModel maps an OpenAI-facing model id to the underlying ChatGPT slug.
// gizmo ids (g-*, g-p-*) collapse to "auto".
func MapModel(model string) string {
	if extractGizmoID(model) != "" {
		return "auto"
	}
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "o3-mini-high"):
		return "o3-mini-high"
	case strings.Contains(lower, "o3-mini"):
		return "o3-mini"
	case strings.Contains(lower, "o1-preview"):
		return "o1-preview"
	case strings.Contains(lower, "o1-pro"):
		return "o1-pro"
	case strings.Contains(lower, "o1-mini"):
		return "o1-mini"
	case strings.Contains(lower, "o1"):
		return "o1"
	case strings.Contains(lower, "gpt-5-3-mini"):
		return "gpt-5-3-mini"
	case strings.Contains(lower, "gpt-5-3"):
		return "gpt-5-3"
	case strings.Contains(lower, "gpt-5-2"):
		return "gpt-5-2"
	case strings.Contains(lower, "gpt-5-1"):
		return "gpt-5-1"
	case strings.Contains(lower, "gpt-5-mini"):
		return "gpt-5-mini"
	case strings.Contains(lower, "gpt-5"):
		return "gpt-5"
	case strings.Contains(lower, "gpt-4o-mini"):
		return "gpt-4o-mini"
	case strings.Contains(lower, "gpt-4o"):
		return "gpt-4o"
	case strings.Contains(lower, "gpt-4"):
		return "gpt-4"
	case strings.Contains(lower, "gpt-3.5"), strings.Contains(lower, "3.5"):
		return "text-davinci-002-render-sha"
	case strings.Contains(lower, "auto"):
		return "auto"
	default:
		return "gpt-4o"
	}
}
