package chatgpt

import (
	"encoding/json"
	"strings"
)

type pendingApproval struct {
	TargetMessageID string
	OperationHash   string
	AlwaysAllow     bool
}

func parsePendingApproval(raw map[string]any) *pendingApproval {
	msg := streamMessageFromRaw(raw)
	if msg == nil {
		return nil
	}
	meta, _ := msg["metadata"].(map[string]any)
	if meta == nil {
		return nil
	}
	jit, _ := meta["jit_plugin_data"].(map[string]any)
	if jit == nil {
		return nil
	}
	fromServer, _ := jit["from_server"].(map[string]any)
	if fromServer == nil {
		return nil
	}
	if strings.TrimSpace(strVal(fromServer["type"])) != "confirm_action" {
		return nil
	}
	msgID, _ := msg["id"].(string)
	if msgID == "" {
		return nil
	}
	operationHash := extractOperationHash(fromServer)
	return &pendingApproval{
		TargetMessageID: msgID,
		OperationHash:   operationHash,
		AlwaysAllow:     true,
	}
}

func extractOperationHash(fromServer map[string]any) string {
	body, _ := fromServer["body"].(map[string]any)
	if body == nil {
		return ""
	}
	if op, ok := body["operation"].(string); ok && op != "" {
		return op
	}
	return operationHashFromActions(body["actions"])
}

func operationHashFromActions(actionsRaw any) string {
	actions, ok := actionsRaw.([]any)
	if !ok {
		return ""
	}
	for _, a := range actions {
		action, _ := a.(map[string]any)
		if action == nil {
			continue
		}
		always, _ := action["always_allow"].(map[string]any)
		if h, ok := always["operation_hash"].(string); ok && h != "" {
			return h
		}
	}
	return ""
}

func buildApprovalMessage(approval pendingApproval) map[string]any {
	approveType := "allow"
	if approval.AlwaysAllow {
		approveType = "always_allow"
	}
	data := map[string]any{"type": approveType}
	if approval.OperationHash != "" {
		data["operation_hash"] = approval.OperationHash
	}
	return map[string]any{
		"id":     newUUID(),
		"author": map[string]string{"role": "user"},
		"content": map[string]any{
			"content_type": "text",
			"parts":        []string{""},
		},
		"metadata": map[string]any{
			"jit_plugin_data": map[string]any{
				"from_client": map[string]any{
					"type":              approveType,
					"target_message_id": approval.TargetMessageID,
					"user_action": map[string]any{
						"target_message_id": approval.TargetMessageID,
						"data":              data,
					},
				},
			},
		},
	}
}

func parseSSEPayload(line string) (map[string]any, bool) {
	if !strings.HasPrefix(line, sseDataPrefix) {
		return nil, false
	}
	payload := strings.TrimSpace(line[6:])
	if payload == "" || payload == "[DONE]" {
		return nil, false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return nil, false
	}
	return raw, true
}