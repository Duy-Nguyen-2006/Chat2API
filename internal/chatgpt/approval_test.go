package chatgpt

import "testing"

func TestParsePendingApproval(t *testing.T) {
	raw := map[string]any{
		"message": map[string]any{
			"id": "msg-123",
			"metadata": map[string]any{
				"jit_plugin_data": map[string]any{
					"from_server": map[string]any{
						"type": "confirm_action",
						"body": map[string]any{
							"operation": "abc123hash",
							"domain":    "example.com",
						},
					},
				},
			},
		},
	}
	got := parsePendingApproval(raw)
	if got == nil {
		t.Fatal("expected approval")
	}
	if got.TargetMessageID != "msg-123" {
		t.Fatalf("message id = %q", got.TargetMessageID)
	}
	if got.OperationHash != "abc123hash" {
		t.Fatalf("operation hash = %q", got.OperationHash)
	}
	if !got.AlwaysAllow {
		t.Fatal("expected always allow default")
	}
}

func TestBuildApprovalMessageAlwaysAllow(t *testing.T) {
	msg := buildApprovalMessage(pendingApproval{
		TargetMessageID: "msg-1",
		OperationHash:   "hash-1",
		AlwaysAllow:     true,
	})
	meta, _ := msg["metadata"].(map[string]any)
	jit, _ := meta["jit_plugin_data"].(map[string]any)
	fromClient, _ := jit["from_client"].(map[string]any)
	if fromClient["type"] != "always_allow" {
		t.Fatalf("type = %v", fromClient["type"])
	}
}