package chatgpt

import "strings"

const (
	modelGPTImage2      = "gpt-image-2"
	modelCodexGPTImage2 = "codex-gpt-image-2"
	modelGPT5           = "gpt-5"
	modelGPT51          = "gpt-5-1"
	modelGPT52          = "gpt-5-2"
	modelGPT53          = "gpt-5-3"
	modelGPT53Mini      = "gpt-5-3-mini"
	modelGPT5Mini       = "gpt-5-mini"
	modelGPT4o          = "gpt-4o"
	modelGPT4oMini      = "gpt-4o-mini"
	modelGPT4           = "gpt-4"
	modelGPT35Turbo     = "gpt-3.5-turbo"
	modelO1             = "o1"
	modelO1Pro          = "o1-pro"
	modelO1Mini         = "o1-mini"
	modelO1Preview      = "o1-preview"
	modelO3Mini         = "o3-mini"
	modelO3MiniHigh     = "o3-mini-high"
	modelAuto           = "auto"
	modelDavinciRender  = "text-davinci-002-render-sha"
)

// SupportedModels lists the model ids exposed via /v1/models. Includes
// text chat models and image-capable aliases (gpt-image-2, codex-gpt-image-2).
// Order is significant: MapModel walks it via string-search precedence, so
// the more-specific aliases (o3-mini-high) must precede the less-specific
// (o3-mini, o1, ...).
var SupportedModels = []string{
	// Image generation.
	modelGPTImage2,
	modelCodexGPTImage2,

	// GPT-5 family (newest).
	modelGPT5,
	modelGPT51,
	modelGPT52,
	modelGPT53,
	modelGPT53Mini,
	modelGPT5Mini,

	// GPT-4o family.
	modelGPT4o,
	modelGPT4oMini,
	modelGPT4,
	modelGPT35Turbo,

	// Reasoning.
	modelO1,
	modelO1Pro,
	modelO1Mini,
	modelO1Preview,
	modelO3Mini,
	modelO3MiniHigh,

	// Search / catch-all.
	modelAuto,
}

// ImageModelIDs returns the subset of SupportedModels usable with the image
// generation pipeline.
func ImageModelIDs() []string {
	out := make([]string, 0, 2)
	for _, m := range SupportedModels {
		switch m {
		case modelGPTImage2, modelCodexGPTImage2:
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
	return lower == modelGPTImage2 || lower == modelCodexGPTImage2
}

// MapModel maps an OpenAI-facing model id to the underlying ChatGPT slug.
// gizmo ids (g-*, g-p-*) collapse to "auto".
func MapModel(model string) string {
	if extractGizmoID(model) != "" {
		return modelAuto
	}
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, modelO3MiniHigh):
		return modelO3MiniHigh
	case strings.Contains(lower, modelO3Mini):
		return modelO3Mini
	case strings.Contains(lower, modelO1Preview):
		return modelO1Preview
	case strings.Contains(lower, modelO1Pro):
		return modelO1Pro
	case strings.Contains(lower, modelO1Mini):
		return modelO1Mini
	case strings.Contains(lower, modelO1):
		return modelO1
	case strings.Contains(lower, modelGPT53Mini):
		return modelGPT53Mini
	case strings.Contains(lower, modelGPT53):
		return modelGPT53
	case strings.Contains(lower, modelGPT52):
		return modelGPT52
	case strings.Contains(lower, modelGPT51):
		return modelGPT51
	case strings.Contains(lower, modelGPT5Mini):
		return modelGPT5Mini
	case strings.Contains(lower, modelGPT5):
		return modelGPT5
	case strings.Contains(lower, modelGPT4oMini):
		return modelGPT4oMini
	case strings.Contains(lower, modelGPT4o):
		return modelGPT4o
	case strings.Contains(lower, modelGPT4):
		return modelGPT4
	case strings.Contains(lower, "gpt-3.5"), strings.Contains(lower, "3.5"):
		return modelDavinciRender
	case strings.Contains(lower, modelAuto):
		return modelAuto
	default:
		return modelGPT4o
	}
}
