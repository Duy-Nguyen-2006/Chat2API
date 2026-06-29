package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func printStreamSummary(line string) {
	if !strings.HasPrefix(line, "data: ") {
		return
	}
	payload := strings.TrimSpace(line[6:])
	if payload == "[DONE]" {
		fmt.Println(line)
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		fmt.Println(line)
		return
	}
	msg, _ := raw["message"].(map[string]any)
	if msg == nil {
		printNonMessageEvent(raw)
		return
	}
	printMessageSummary(raw, msg)
}

func printNonMessageEvent(raw map[string]any) {
	if raw["type"] == nil {
		return
	}
	b, _ := json.Marshal(raw)
	if len(b) > 500 {
		b = append(b[:500], []byte("...")...)
	}
	fmt.Printf("event=%s\n", string(b))
}

func printMessageSummary(raw, msg map[string]any) {
	author, _ := msg["author"].(map[string]any)
	content, _ := msg["content"].(map[string]any)
	meta, _ := msg["metadata"].(map[string]any)
	fmt.Printf("type=%v status=%v role=%v recipient=%v content_type=%v end_turn=%v\n",
		raw["type"], msg["status"], author["role"], msg["recipient"],
		content["content_type"], msg["end_turn"])
	printMessageMeta(meta)
	printMessageParts(content)
	printMessageText(content)
}

func printMessageMeta(meta map[string]any) {
	if meta == nil {
		return
	}
	for _, k := range []string{
		"command", "finished_text", "initial_text", "aggregate_result",
		"tool_invoked", "tool_name", "invoked_plugin", "invoked_resource",
		"jit_plugin_data", "request_id", "permissions", "approval",
	} {
		if v, ok := meta[k]; ok {
			b, _ := json.Marshal(v)
			if len(b) > 2 {
				fmt.Printf("  meta.%s=%s\n", k, string(b))
			}
		}
	}
}

func printMessageParts(content map[string]any) {
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) == 0 {
		return
	}
	b, _ := json.Marshal(parts)
	if len(b) > 300 {
		b = append(b[:300], []byte("...")...)
	}
	fmt.Printf("  parts=%s\n", string(b))
}

func printMessageText(content map[string]any) {
	text, ok := content["text"].(string)
	if !ok || text == "" {
		return
	}
	preview := text
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	fmt.Printf("  text=%q\n", preview)
}