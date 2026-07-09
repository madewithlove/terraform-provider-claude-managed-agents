package client

import (
	"context"
	"net/url"
)

// Packages pre-installs dependencies in a cloud sandbox, keyed by package
// manager. Each value is a list of (optionally version-pinned) package names.
type Packages struct {
	Apt   []string `json:"apt,omitempty"`
	Cargo []string `json:"cargo,omitempty"`
	Gem   []string `json:"gem,omitempty"`
	Go    []string `json:"go,omitempty"`
	Npm   []string `json:"npm,omitempty"`
	Pip   []string `json:"pip,omitempty"`
}

// Networking controls outbound network access for a cloud sandbox.
// Type is "unrestricted" (default) or "limited". The allow_* flags and
// allowed_hosts only apply to "limited".
type Networking struct {
	Type                 string   `json:"type"`
	AllowedHosts         []string `json:"allowed_hosts,omitempty"`
	AllowMCPServers      *bool    `json:"allow_mcp_servers,omitempty"`
	AllowPackageManagers *bool    `json:"allow_package_managers,omitempty"`
}

// EnvironmentConfig describes where sessions run. Type is "cloud" or
// "self_hosted". Packages/Networking apply to cloud environments.
type EnvironmentConfig struct {
	Type       string      `json:"type"`
	Packages   *Packages   `json:"packages,omitempty"`
	Networking *Networking `json:"networking,omitempty"`
}

// Environment is the API representation of an environment.
type Environment struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Config     EnvironmentConfig `json:"config"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
	ArchivedAt *string           `json:"archived_at"`
}

// EnvironmentCreateRequest is the POST /v1/environments body.
type EnvironmentCreateRequest struct {
	Name   string            `json:"name"`
	Config EnvironmentConfig `json:"config"`
}

// CreateEnvironment creates an environment.
func (c *Client) CreateEnvironment(ctx context.Context, req EnvironmentCreateRequest) (*Environment, error) {
	var out Environment
	if err := c.do(ctx, "POST", "/v1/environments", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetEnvironment fetches an environment by ID.
func (c *Client) GetEnvironment(ctx context.Context, id string) (*Environment, error) {
	var out Environment
	if err := c.do(ctx, "GET", "/v1/environments/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteEnvironment hard-deletes an environment. Fails (409) if any session
// still references it.
func (c *Client) DeleteEnvironment(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/v1/environments/"+url.PathEscape(id), nil, nil)
}

// ArchiveEnvironment makes an environment read-only. Existing sessions keep
// running; new sessions cannot reference it.
func (c *Client) ArchiveEnvironment(ctx context.Context, id string) (*Environment, error) {
	var out Environment
	if err := c.do(ctx, "POST", "/v1/environments/"+url.PathEscape(id)+"/archive", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
