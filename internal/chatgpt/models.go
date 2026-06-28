package chatgpt

import "strings"

var SupportedModels = []string{
	"gpt-4o",
	"gpt-4o-mini",
	"gpt-4",
	"gpt-3.5-turbo",
	"o1",
	"o1-mini",
	"o1-preview",
	"o3-mini",
	"auto",
}

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