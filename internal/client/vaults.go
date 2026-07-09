package client

import (
	"context"
	"encoding/json"
	"net/url"
)

// Vault is a workspace-scoped collection of credentials for one end user.
type Vault struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	DisplayName string            `json:"display_name"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	ArchivedAt  *string           `json:"archived_at"`
}

// VaultCreateRequest is the POST /v1/vaults body.
type VaultCreateRequest struct {
	DisplayName string            `json:"display_name"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// VaultUpdateRequest is the POST /v1/vaults/{id} body. Metadata is a patch:
// a string value upserts a key, a nil value deletes it, omitted keys persist.
type VaultUpdateRequest struct {
	DisplayName *string                `json:"display_name,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (c *Client) CreateVault(ctx context.Context, req VaultCreateRequest) (*Vault, error) {
	var out Vault
	if err := c.do(ctx, "POST", "/v1/vaults", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetVault(ctx context.Context, id string) (*Vault, error) {
	var out Vault
	if err := c.do(ctx, "GET", "/v1/vaults/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateVault(ctx context.Context, id string, req VaultUpdateRequest) (*Vault, error) {
	var out Vault
	if err := c.do(ctx, "POST", "/v1/vaults/"+url.PathEscape(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ArchiveVault archives a vault (cascades to its credentials; secrets purged,
// records retained for auditing).
func (c *Client) ArchiveVault(ctx context.Context, id string) (*Vault, error) {
	var out Vault
	if err := c.do(ctx, "POST", "/v1/vaults/"+url.PathEscape(id)+"/archive", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteVault hard-deletes a vault (no audit trail retained).
func (c *Client) DeleteVault(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/v1/vaults/"+url.PathEscape(id), nil, nil)
}

// InjectionLocation scopes where an environment-variable secret is substituted.
type InjectionLocation struct {
	Header bool `json:"header"`
	Body   bool `json:"body"`
}

// CredentialNetworking scopes which outbound hosts a secret is substituted on.
type CredentialNetworking struct {
	Type         string   `json:"type"`
	AllowedHosts []string `json:"allowed_hosts,omitempty"`
}

// CredentialRefreshResponse is the non-secret refresh config returned in
// credential responses (tokens and client secrets are never returned).
type CredentialRefreshResponse struct {
	ClientID          string  `json:"client_id"`
	TokenEndpoint     string  `json:"token_endpoint"`
	Scope             *string `json:"scope"`
	Resource          *string `json:"resource"`
	TokenEndpointAuth struct {
		Type string `json:"type"`
	} `json:"token_endpoint_auth"`
}

// CredentialAuthResponse is the non-secret auth view returned by the API.
type CredentialAuthResponse struct {
	Type              string                     `json:"type"`
	MCPServerURL      string                     `json:"mcp_server_url,omitempty"`
	ExpiresAt         *string                    `json:"expires_at,omitempty"`
	Refresh           *CredentialRefreshResponse `json:"refresh,omitempty"`
	SecretName        string                     `json:"secret_name,omitempty"`
	Networking        *CredentialNetworking      `json:"networking,omitempty"`
	InjectionLocation *InjectionLocation         `json:"injection_location,omitempty"`
}

// Credential is a credential stored in a vault. Sensitive fields are never
// returned in responses.
type Credential struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	VaultID     string                 `json:"vault_id"`
	DisplayName *string                `json:"display_name"`
	Metadata    map[string]string      `json:"metadata"`
	Auth        CredentialAuthResponse `json:"auth"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
	ArchivedAt  *string                `json:"archived_at"`
}

// CredentialCreateRequest is the POST /v1/vaults/{vault_id}/credentials body.
// Auth is the raw auth union (built by the provider).
type CredentialCreateRequest struct {
	DisplayName *string           `json:"display_name,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Auth        json.RawMessage   `json:"auth"`
}

// CredentialUpdateRequest is the POST /v1/vaults/{vault_id}/credentials/{id}
// body. Auth carries only the mutable subset (structural keys are immutable).
type CredentialUpdateRequest struct {
	DisplayName *string                `json:"display_name,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Auth        json.RawMessage        `json:"auth,omitempty"`
}

func (c *Client) CreateCredential(ctx context.Context, vaultID string, req CredentialCreateRequest) (*Credential, error) {
	var out Credential
	if err := c.do(ctx, "POST", "/v1/vaults/"+url.PathEscape(vaultID)+"/credentials", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetCredential(ctx context.Context, vaultID, credID string) (*Credential, error) {
	var out Credential
	if err := c.do(ctx, "GET", "/v1/vaults/"+url.PathEscape(vaultID)+"/credentials/"+url.PathEscape(credID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateCredential(ctx context.Context, vaultID, credID string, req CredentialUpdateRequest) (*Credential, error) {
	var out Credential
	if err := c.do(ctx, "POST", "/v1/vaults/"+url.PathEscape(vaultID)+"/credentials/"+url.PathEscape(credID), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ArchiveCredential archives a credential (secret purged; key freed for reuse).
func (c *Client) ArchiveCredential(ctx context.Context, vaultID, credID string) (*Credential, error) {
	var out Credential
	if err := c.do(ctx, "POST", "/v1/vaults/"+url.PathEscape(vaultID)+"/credentials/"+url.PathEscape(credID)+"/archive", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteCredential hard-deletes a credential.
func (c *Client) DeleteCredential(ctx context.Context, vaultID, credID string) error {
	return c.do(ctx, "DELETE", "/v1/vaults/"+url.PathEscape(vaultID)+"/credentials/"+url.PathEscape(credID), nil, nil)
}
