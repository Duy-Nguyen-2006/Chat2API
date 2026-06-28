package chatgpt

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// respFromString builds an *http.Response with a fixed body. Helper for
// testing DecodeWorkspaceList / DecodeGizmoList without hitting the network.
func respFromString(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDecodeWorkspaceList_Basic(t *testing.T) {
	body := `{
	  "accounts": {
	    "default": {
	      "account": {"account_id":"acc-default","structure":"personal","plan_type":"free"},
	      "can_access_with_session": true
	    },
	    "ws-1": {
	      "account": {"account_id":"ws-1","structure":"workspace","plan_type":"team","name":"Acme"},
	      "can_access_with_session": true
	    },
	    "ws-2": {
	      "account": {"account_id":"ws-2","structure":"workspace","plan_type":"free","is_deactivated":true},
	      "can_access_with_session": false
	    }
	  }
	}`
	list, err := DecodeWorkspaceList(respFromString(body))
	if err != nil {
		t.Fatal(err)
	}
	if list.Object != "list" {
		t.Errorf("object: %q", list.Object)
	}
	if len(list.Data) != 2 {
		t.Fatalf("expected 2 workspaces (excluding default), got %d", len(list.Data))
	}
	byID := map[string]Workspace{}
	for _, w := range list.Data {
		byID[w.ID] = w
	}
	if w, ok := byID["ws-1"]; !ok {
		t.Error("missing ws-1")
	} else if w.Title != "Acme" || !w.CanAccess || w.Plan != "team" {
		t.Errorf("ws-1: %+v", w)
	}
	if w, ok := byID["ws-2"]; !ok {
		t.Error("missing ws-2")
	} else if !w.Deactivated || w.CanAccess || w.Title == "Acme" {
		t.Errorf("ws-2 unexpected: %+v", w)
	}
}

func TestDecodeWorkspaceList_PersonalFallback(t *testing.T) {
	body := `{"accounts":{"p1":{"account":{"account_id":"p1","structure":"personal","plan_type":"free"},"can_access_with_session":true}}}`
	list, err := DecodeWorkspaceList(respFromString(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].Title != "Personal" {
		t.Errorf("title fallback: %+v", list.Data)
	}
}

func TestDecodeWorkspaceList_BadJSON(t *testing.T) {
	if _, err := DecodeWorkspaceList(respFromString("not json")); err == nil {
		t.Error("expected decode error")
	}
}

func TestDecodeWorkspaceList_EmptyAccounts(t *testing.T) {
	list, err := DecodeWorkspaceList(respFromString(`{"accounts":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 0 {
		t.Errorf("expected empty data, got %+v", list.Data)
	}
}