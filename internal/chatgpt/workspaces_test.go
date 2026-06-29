package chatgpt

import "testing"

func TestWorkspacesFromCheck_OrderingAndDeactivated(t *testing.T) {
	name := "ChatGPT Business"
	check := accountsCheckResponse{
		AccountOrdering: []string{"ws-1", "ws-2", "ws-3"},
		Accounts: map[string]accountsCheckEntry{
			"ws-1": {Account: struct {
				AccountID      string  `json:"account_id"`
				OrganizationID *string `json:"organization_id"`
				Name           *string `json:"name"`
				Structure      string  `json:"structure"`
				PlanType       string  `json:"plan_type"`
				IsDeactivated  bool    `json:"is_deactivated"`
			}{AccountID: "ws-1", Name: strPtr("Duy"), Structure: "workspace", PlanType: "k12"}, CanAccessWithSession: true},
			"ws-2": {Account: struct {
				AccountID      string  `json:"account_id"`
				OrganizationID *string `json:"organization_id"`
				Name           *string `json:"name"`
				Structure      string  `json:"structure"`
				PlanType       string  `json:"plan_type"`
				IsDeactivated  bool    `json:"is_deactivated"`
			}{AccountID: "ws-2", Structure: "personal", PlanType: "free"}, CanAccessWithSession: true},
			"ws-3": {Account: struct {
				AccountID      string  `json:"account_id"`
				OrganizationID *string `json:"organization_id"`
				Name           *string `json:"name"`
				Structure      string  `json:"structure"`
				PlanType       string  `json:"plan_type"`
				IsDeactivated  bool    `json:"is_deactivated"`
			}{AccountID: "ws-3", Name: &name, Structure: "workspace", PlanType: "team", IsDeactivated: true}, CanAccessWithSession: true},
		},
	}
	list := workspacesFromCheck(check)
	if len(list) != 3 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].ID != "ws-1" || list[1].ID != "ws-2" || list[2].ID != "ws-3" {
		t.Fatalf("order: %+v", list)
	}
	if !list[2].Deactivated || list[2].Title != "ChatGPT Business" {
		t.Fatalf("deactivated ws: %+v", list[2])
	}
	if list[1].Title != "Personal" {
		t.Fatalf("personal title: %q", list[1].Title)
	}
}

func strPtr(s string) *string { return &s }

func TestFallbackWorkspaceList(t *testing.T) {
	list := FallbackWorkspaceList("acc-123", "alice@example.com")
	if list.CurrentAccountID != "acc-123" {
		t.Fatalf("current: %q", list.CurrentAccountID)
	}
	if len(list.Data) != 1 {
		t.Fatalf("len: %d", len(list.Data))
	}
	ws := list.Data[0]
	if ws.ID != "acc-123" || ws.Title != "alice@example.com" || !ws.Personal || !ws.CanAccess {
		t.Fatalf("workspace: %+v", ws)
	}
}