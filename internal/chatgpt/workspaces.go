package chatgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const accountsCheckPath = "/backend-api/accounts/check/v4-2023-04-27"

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
	Accounts map[string]struct {
		Account struct {
			AccountID      string  `json:"account_id"`
			OrganizationID *string `json:"organization_id"`
			Name           *string `json:"name"`
			Structure      string  `json:"structure"`
			PlanType       string  `json:"plan_type"`
			IsDeactivated  bool    `json:"is_deactivated"`
		} `json:"account"`
		CanAccessWithSession bool `json:"can_access_with_session"`
	} `json:"accounts"`
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
	req.Header = c.buildHeaders(nil)
	return c.http.Do(req)
}

// DecodeWorkspaceList converts an accounts-check response into the public
// WorkspaceList shape. The caller must close resp.Body afterwards.
func DecodeWorkspaceList(resp *http.Response) (WorkspaceList, error) {
	var check accountsCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&check); err != nil {
		return WorkspaceList{}, err
	}
	workspaces := make([]Workspace, 0, len(check.Accounts))
	for key, entry := range check.Accounts {
		if key == "default" {
			continue
		}

		acc := entry.Account
		accountID := acc.AccountID
		if accountID == "" {
			accountID = key
		}

		title := ""
		if acc.Name != nil {
			title = *acc.Name
		}
		if title == "" {
			if acc.Structure == "personal" {
				title = "Personal"
			} else {
				title = accountID
			}
		}

		orgID := ""
		if acc.OrganizationID != nil {
			orgID = *acc.OrganizationID
		}

		workspaces = append(workspaces, Workspace{
			ID:          accountID,
			OrgID:       orgID,
			Title:       title,
			Structure:   acc.Structure,
			Plan:        acc.PlanType,
			Personal:    acc.Structure == "personal",
			Deactivated: acc.IsDeactivated,
			CanAccess:   entry.CanAccessWithSession,
		})
	}

	return WorkspaceList{
		Object: "list",
		Data:   workspaces,
	}, nil
}

// ListWorkspaces is a convenience wrapper around ListWorkspacesRaw + decode.
// Retained for callers that don't need retry control.
func (c *Client) ListWorkspaces(ctx context.Context) (WorkspaceList, error) {
	resp, err := c.ListWorkspacesRaw(ctx)
	if err != nil {
		return WorkspaceList{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return WorkspaceList{}, fmt.Errorf("accounts/check HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}
	list, err := DecodeWorkspaceList(resp)
	if err != nil {
		return WorkspaceList{}, err
	}
	// Backfill CurrentAccountID with the client's selection so callers that
	// already migrated to ListWorkspaces keep getting the same shape.
	list.CurrentAccountID = c.accountID
	for i := range list.Data {
		list.Data[i].IsDefault = list.Data[i].ID == c.accountID
		list.Data[i].IsCurrent = list.Data[i].ID == c.accountID
	}
	return list, nil
}

func (c *Client) WithAccountID(accountID string) *Client {
	if accountID == "" || accountID == c.accountID {
		return c
	}
	clone := *c
	clone.accountID = accountID
	return &clone
}