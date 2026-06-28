package chatgpt

import (
	"strings"
	"testing"
)

func TestRelayConversationStream_Done(t *testing.T) {
	in := "data: [DONE]\n\n"
	res, err := relayConversationStream(strings.NewReader(in), &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.done {
		t.Error("expected done=true")
	}
}

func TestRelayConversationStream_ConversationID(t *testing.T) {
	in := "data: {\"conversation_id\":\"abc-123\",\"other\":1}\n\n"
	res, err := relayConversationStream(strings.NewReader(in), &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if res.conversationID != "abc-123" {
		t.Errorf("got %q", res.conversationID)
	}
	if res.done {
		t.Error("should not be done")
	}
	if res.approval != nil {
		t.Error("should not have approval")
	}
}

func TestRelayConversationStream_Approval(t *testing.T) {
	payload := `{"message":{"id":"msg-1","metadata":{"jit_plugin_data":{"from_server":{"type":"confirm_action","body":{"operation":"op-hash-1"}}}}}}`
	in := "data: " + payload + "\n\n"
	res, err := relayConversationStream(strings.NewReader(in), &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if res.approval == nil {
		t.Fatal("expected approval")
	}
	if res.approval.TargetMessageID != "msg-1" {
		t.Errorf("target: %q", res.approval.TargetMessageID)
	}
	if res.approval.OperationHash != "op-hash-1" {
		t.Errorf("op hash: %q", res.approval.OperationHash)
	}
	if !res.approval.AlwaysAllow {
		t.Error("AlwaysAllow should default to true")
	}
}

func TestRelayConversationStream_SkipsNonDataLines(t *testing.T) {
	in := "event: message\n: comment\ndata: {\"conversation_id\":\"x\"}\n\n"
	res, err := relayConversationStream(strings.NewReader(in), &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if res.conversationID != "x" {
		t.Errorf("got %q", res.conversationID)
	}
}