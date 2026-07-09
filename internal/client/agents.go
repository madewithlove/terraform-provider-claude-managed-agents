package client

import (
	"context"
	"encoding/json"
	"net/url"
)

// ModelConfig is the agent's model selection. The API also accepts a bare
// string, but we always send/receive the object form.
type ModelConfig struct {
	ID    string `json:"id"`
	Speed string `json:"speed,omitempty"`
}

// Agent is the API representation of an agent (a specific version).
type Agent struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Model       ModelConfig       `json:"model"`
	System      *string           `json:"system"`
	Description *string           `json:"description"`
	Tools       json.RawMessage   `json:"tools"`
	Skills      json.RawMessage   `json:"skills"`
	MCPServers  json.RawMessage   `json:"mcp_servers"`
	Multiagent  json.RawMessage   `json:"multiagent"`
	Metadata    map[string]string `json:"metadata"`
	Version     int64             `json:"version"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	ArchivedAt  *string           `json:"archived_at"`
}

// AgentCreateRequest is the POST /v1/agents body. Optional fields use
// omitempty so unset values are not transmitted.
type AgentCreateRequest struct {
	Name        string            `json:"name"`
	Model       ModelConfig       `json:"model"`
	System      *string           `json:"system,omitempty"`
	Description *string           `json:"description,omitempty"`
	Tools       json.RawMessage   `json:"tools,omitempty"`
	Skills      json.RawMessage   `json:"skills,omitempty"`
	MCPServers  json.RawMessage   `json:"mcp_servers,omitempty"`
	Multiagent  json.RawMessage   `json:"multiagent,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// AgentUpdateRequest is the POST /v1/agents/{id} body. Version is required
// for optimistic concurrency. Nullable scalar/array fields are sent
// unconditionally so that clearing them in config clears them server-side.
type AgentUpdateRequest struct {
	Version     int64                  `json:"version"`
	Name        string                 `json:"name"`
	Model       ModelConfig            `json:"model"`
	System      *string                `json:"system"`
	Description *string                `json:"description"`
	Tools       json.RawMessage        `json:"tools"`
	Skills      json.RawMessage        `json:"skills"`
	MCPServers  json.RawMessage        `json:"mcp_servers"`
	Multiagent  json.RawMessage        `json:"multiagent"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreateAgent creates a new agent (version 1).
func (c *Client) CreateAgent(ctx context.Context, req AgentCreateRequest) (*Agent, error) {
	var out Agent
	if err := c.do(ctx, "POST", "/v1/agents", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAgent fetches the current version of an agent.
func (c *Client) GetAgent(ctx context.Context, id string) (*Agent, error) {
	var out Agent
	if err := c.do(ctx, "GET", "/v1/agents/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAgent updates an agent. A version mismatch returns a 409 (IsConflict).
func (c *Client) UpdateAgent(ctx context.Context, id string, req AgentUpdateRequest) (*Agent, error) {
	var out Agent
	if err := c.do(ctx, "POST", "/v1/agents/"+url.PathEscape(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ArchiveAgent archives an agent. Agents cannot be hard-deleted; archiving
// is the terminal operation (existing sessions keep running).
func (c *Client) ArchiveAgent(ctx context.Context, id string) (*Agent, error) {
	var out Agent
	if err := c.do(ctx, "POST", "/v1/agents/"+url.PathEscape(id)+"/archive", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
