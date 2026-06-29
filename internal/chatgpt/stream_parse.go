package chatgpt

import "strings"

func streamMessageFromRaw(raw map[string]any) map[string]any {
	if msg, ok := raw["message"].(map[string]any); ok {
		return msg
	}
	if v, ok := raw["v"].(map[string]any); ok {
		if msg, ok := v["message"].(map[string]any); ok {
			return msg
		}
	}
	if msg, ok := raw["input_message"].(map[string]any); ok {
		return msg
	}
	return nil
}

func conversationIDFromRaw(raw map[string]any) string {
	if cid, _ := raw["conversation_id"].(string); cid != "" {
		return cid
	}
	if v, ok := raw["v"].(map[string]any); ok {
		if cid, _ := v["conversation_id"].(string); cid != "" {
			return cid
		}
	}
	return ""
}

func extractPatchFragments(raw map[string]any) string {
	if frag := appendFragmentFromPatch(raw); frag != "" {
		return frag
	}
	if strings.TrimSpace(strVal(raw["o"])) != "patch" {
		return ""
	}
	patches, ok := raw["v"].([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, item := range patches {
		patch, _ := item.(map[string]any)
		if patch == nil {
			continue
		}
		if s := appendFragmentFromPatch(patch); s != "" {
			b.WriteString(s)
		}
	}
	return b.String()
}

func appendFragmentFromPatch(patch map[string]any) string {
	if strVal(patch["o"]) != "append" {
		return ""
	}
	path := strVal(patch["p"])
	if !strings.Contains(path, "content/parts") {
		return ""
	}
	s, _ := patch["v"].(string)
	return s
}

func messageTextFromMessage(msg map[string]any) string {
	if msg == nil {
		return ""
	}
	author, _ := msg["author"].(map[string]any)
	role := strVal(author["role"])
	if role == "user" || role == "system" {
		return ""
	}
	content, _ := msg["content"].(map[string]any)
	if content == nil {
		return ""
	}
	if text, ok := content["text"].(string); ok && text != "" {
		return text
	}
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if s, ok := p.(string); ok {
			b.WriteString(s)
		}
	}
	return b.String()
}

func streamChunkFromLine(line string, accumulated *string) string {
	raw, ok := parseSSEPayload(line)
	if !ok {
		return ""
	}
	if frag := extractPatchFragments(raw); frag != "" {
		*accumulated += frag
		return frag
	}
	delta := messageTextFromMessage(streamMessageFromRaw(raw))
	if delta == "" {
		return ""
	}
	if len(delta) <= len(*accumulated) {
		return ""
	}
	chunk := delta[len(*accumulated):]
	*accumulated = delta
	return chunk
}