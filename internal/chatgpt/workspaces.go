package chatgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	accountsCheckPath = "/backend-api/accounts/check/v4-2023-04-27"
	accountsListPath  = "/backend-api/accounts"
)

type Workspace struct {
	ID          string `json:"id"`
	OrgID       string `json:"org_id,omitempty"`
	Title       string `json:"title"`
	Structure   string `json:"structure"`
	Plan        string `json:"plan,omitempty"`
	Personal    bool   `json:"personal"`
	IsDefault   bool   `json:"is_default"`
	IsCurrent   bool   `json:"is_current"`
	Deactivated bool   `json:"deactivated,omitempty"`
	CanAccess   bool   `json:"can_access"`
}

type WorkspaceList struct {
	Object           string      `json:"object"`
	CurrentAccountID string      `json:"current_account_id"`
	Data             []Workspace `json:"data"`
}

type accountsCheckResponse struct {
	Accounts        map[string]accountsCheckEntry `json:"accounts"`
	AccountOrdering []string                      `json:"account_ordering"`
}

type accountsCheckEntry struct {
	Account struct {
		AccountID      string  `json:"account_id"`
		OrganizationID *string `json:"organization_id"`
		Name           *string `json:"name"`
		Structure      string  `json:"structure"`
		PlanType       string  `json:"plan_type"`
		IsDeactivated  bool    `json:"is_deactivated"`
	} `json:"account"`
	CanAccessWithSession bool `json:"can_access_with_session"`
}

type accountsListResponse struct {
	Items []struct {
		ID        string  `json:"id"`
		Name      *string `json:"name"`
		Structure string  `json:"structure"`
	} `json:"items"`
}

// ListWorkspacesRaw performs the accounts-check request and returns the
// upstream HTTP response. The caller is responsible for closing the body
// and checking the status code. Used by the server layer so retries can
// reuse the response before deciding to fall back to a different account.
func (c *Client) ListWorkspacesRaw(ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+accountsCheckPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildAccountListHeaders()
	return c.sessionDo(req)
}

// FallbackWorkspaceList builds a single-workspace list from pool metadata when
// accounts/check is blocked (e.g. Cloudflare) but the session may still work.
func FallbackWorkspaceList(accountID, title string) WorkspaceList {
	if title == "" {
		title = "Personal"
	}
	ws := Workspace{
		ID:        accountID,
		Title:     title,
		Structure: "personal",
		Personal:  true,
		IsDefault: true,
		IsCurrent: true,
		CanAccess: true,
	}
	return WorkspaceList{
		Object:           "list",
		CurrentAccountID: accountID,
		Data:             []Workspace{ws},
	}
}

func workspaceFromEntry(key string, entry accountsCheckEntry) Workspace {
	acc := entry.Account
	accountID := acc.AccountID
	if accountID == "" {
		accountID = key
	}
	title := workspaceTitle(acc.Name, acc.Structure, accountID)
	orgID := ""
	if acc.OrganizationID != nil {
		orgID = *acc.OrganizationID
	}
	return Workspace{
		ID:          accountID,
		OrgID:       orgID,
		Title:       title,
		Structure:   acc.Structure,
		Plan:        acc.PlanType,
		Personal:    acc.Structure == "personal",
		Deactivated: acc.IsDeactivated,
		CanAccess:   entry.CanAccessWithSession,
	}
}

func workspaceTitle(name *string, structure, accountID string) string {
	if name != nil && *name != "" {
		return *name
	}
	if structure == "personal" {
		return "Personal"
	}
	return accountID
}

func workspacesFromCheck(check accountsCheckResponse) []Workspace {
	byID := make(map[string]Workspace, len(check.Accounts))
	order := make([]string, 0, len(check.Accounts))
	for key, entry := range check.Accounts {
		if key == "default" {
			continue
		}
		ws := workspaceFromEntry(key, entry)
		byID[ws.ID] = ws
		order = append(order, ws.ID)
	}
	if len(check.AccountOrdering) > 0 {
		order = check.AccountOrdering
	}
	workspaces := make([]Workspace, 0, len(byID))
	seen := map[string]bool{}
	for _, id := range order {
		if ws, ok := byID[id]; ok && !seen[id] {
			workspaces = append(workspaces, ws)
			seen[id] = true
		}
	}
	for id, ws := range byID {
		if !seen[id] {
			workspaces = append(workspaces, ws)
		}
	}
	return workspaces
}

// DecodeWorkspaceList converts an accounts-check response into the public
// WorkspaceList shape. The caller must close resp.Body afterwards.
func DecodeWorkspaceList(resp *http.Response) (WorkspaceList, error) {
	var check accountsCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&check); err != nil {
		return WorkspaceList{}, err
	}
	return WorkspaceList{
		Object: "list",
		Data:   workspacesFromCheck(check),
	}, nil
}

func decodeWorkspaceListFromAccounts(resp *http.Response) (WorkspaceList, error) {
	var list accountsListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return WorkspaceList{}, err
	}
	workspaces := make([]Workspace, 0, len(list.Items))
	for _, item := range list.Items {
		workspaces = append(workspaces, Workspace{
			ID:        item.ID,
			Title:     workspaceTitle(item.Name, item.Structure, item.ID),
			Structure: item.Structure,
			Personal:  item.Structure == "personal",
			CanAccess: true,
		})
	}
	return WorkspaceList{Object: "list", Data: workspaces}, nil
}

func (c *Client) finalizeWorkspaceList(list WorkspaceList) WorkspaceList {
	list.CurrentAccountID = c.accountID
	for i := range list.Data {
		list.Data[i].IsDefault = list.Data[i].ID == c.accountID
		list.Data[i].IsCurrent = list.Data[i].ID == c.accountID
	}
	return list
}

func (c *Client) getJSON(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildAccountListHeaders()
	return c.sessionDo(req)
}

// ListWorkspaces is a convenience wrapper around ListWorkspacesRaw + decode.
// Retained for callers that don't need retry control.
func (c *Client) ListWorkspaces(ctx context.Context) (WorkspaceList, error) {
	resp, err := c.ListWorkspacesRaw(ctx)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			list, decErr := DecodeWorkspaceList(resp)
			if decErr == nil && len(list.Data) > 0 {
				return c.finalizeWorkspaceList(list), nil
			}
			if decErr != nil {
				err = decErr
			}
		} else {
			err = fmt.Errorf("accounts/check HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
		}
	}

	fallback, fbErr := c.getJSON(ctx, accountsListPath)
	if fbErr == nil {
		defer fallback.Body.Close()
		if fallback.StatusCode == http.StatusOK {
			list, decErr := decodeWorkspaceListFromAccounts(fallback)
			if decErr == nil && len(list.Data) > 0 {
				return c.finalizeWorkspaceList(list), nil
			}
			if fbErr == nil && decErr != nil {
				fbErr = decErr
			}
		} else {
			fbErr = fmt.Errorf("accounts HTTP %d: %s", fallback.StatusCode, readLimitedBody(fallback.Body))
		}
	}
	if err != nil {
		return WorkspaceList{}, err
	}
	return WorkspaceList{}, fbErr
}

func (c *Client) WithAccountID(accountID string) *Client {
	if accountID == "" || accountID == c.accountID {
		return c
	}
	clone := *c
	clone.accountID = accountID
	return &clone
}