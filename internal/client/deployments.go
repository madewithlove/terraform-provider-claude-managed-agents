package client

import (
	"context"
	"encoding/json"
	"net/url"
)

// Schedule is a deployment's recurring cron schedule. Expression is standard
// POSIX cron; Timezone is an IANA identifier. LastRunAt and UpcomingRunsAt are
// populated by the API.
type Schedule struct {
	Type           string   `json:"type"`
	Expression     string   `json:"expression"`
	Timezone       string   `json:"timezone"`
	LastRunAt      *string  `json:"last_run_at,omitempty"`
	UpcomingRunsAt []string `json:"upcoming_runs_at,omitempty"`
}

// PausedReason explains why a deployment is paused (nil when active).
type PausedReason struct {
	Type  string          `json:"type"`
	Error json.RawMessage `json:"error,omitempty"`
}

// Deployment is the API representation of a scheduled deployment.
type Deployment struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Name          string          `json:"name"`
	Status        string          `json:"status"`
	Agent         json.RawMessage `json:"agent"`
	EnvironmentID string          `json:"environment_id"`
	Schedule      Schedule        `json:"schedule"`
	PausedReason  *PausedReason   `json:"paused_reason"`
	CreatedAt     string          `json:"created_at"`
}

// DeploymentCreateRequest is the POST /v1/deployments body. Agent accepts a
// string ID or an object; we send a string. The optional passthrough fields
// carry raw JSON so beta-only shapes work without a code change.
type DeploymentCreateRequest struct {
	Name          string          `json:"name"`
	Agent         string          `json:"agent"`
	EnvironmentID string          `json:"environment_id"`
	InitialEvents json.RawMessage `json:"initial_events"`
	Schedule      Schedule        `json:"schedule"`
	Files         json.RawMessage `json:"files,omitempty"`
	GitHub        json.RawMessage `json:"github,omitempty"`
	MemoryStores  json.RawMessage `json:"memory_stores,omitempty"`
	Vaults        json.RawMessage `json:"vaults,omitempty"`
}

// CreateDeployment creates a scheduled deployment.
func (c *Client) CreateDeployment(ctx context.Context, req DeploymentCreateRequest) (*Deployment, error) {
	var out Deployment
	if err := c.do(ctx, "POST", "/v1/deployments", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetDeployment fetches a deployment by ID.
func (c *Client) GetDeployment(ctx context.Context, id string) (*Deployment, error) {
	var out Deployment
	if err := c.do(ctx, "GET", "/v1/deployments/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PauseDeployment suppresses scheduled triggers going forward.
func (c *Client) PauseDeployment(ctx context.Context, id string) (*Deployment, error) {
	var out Deployment
	if err := c.do(ctx, "POST", "/v1/deployments/"+url.PathEscape(id)+"/pause", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UnpauseDeployment resumes the schedule from the next occurrence.
func (c *Client) UnpauseDeployment(ctx context.Context, id string) (*Deployment, error) {
	var out Deployment
	if err := c.do(ctx, "POST", "/v1/deployments/"+url.PathEscape(id)+"/unpause", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ArchiveDeployment terminally archives a deployment.
func (c *Client) ArchiveDeployment(ctx context.Context, id string) (*Deployment, error) {
	var out Deployment
	if err := c.do(ctx, "POST", "/v1/deployments/"+url.PathEscape(id)+"/archive", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
