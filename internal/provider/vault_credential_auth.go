package provider

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

// buildAuthCreate builds the full auth union for a credential create, reading
// values (including write-only secrets) from the config model.
func buildAuthCreate(ctx context.Context, a *credAuthBlock) (json.RawMessage, diag.Diagnostics) {
	var diags diag.Diagnostics
	if a == nil {
		return nil, diags
	}
	m := map[string]interface{}{"type": a.Type.ValueString()}

	switch a.Type.ValueString() {
	case "mcp_oauth":
		m["mcp_server_url"] = a.MCPServerURL.ValueString()
		putStr(m, "access_token", a.AccessToken)
		putStr(m, "expires_at", a.ExpiresAt)
		if a.Refresh != nil {
			rf := map[string]interface{}{
				"token_endpoint": a.Refresh.TokenEndpoint.ValueString(),
				"client_id":      a.Refresh.ClientID.ValueString(),
			}
			putStr(rf, "scope", a.Refresh.Scope)
			putStr(rf, "refresh_token", a.Refresh.RefreshToken)
			if tea := a.Refresh.TokenEndpointAuth; tea != nil && !tea.Type.IsNull() && !tea.Type.IsUnknown() {
				teaM := map[string]interface{}{"type": tea.Type.ValueString()}
				putStr(teaM, "client_secret", tea.ClientSecret)
				rf["token_endpoint_auth"] = teaM
			}
			m["refresh"] = rf
		}
	case "static_bearer":
		m["mcp_server_url"] = a.MCPServerURL.ValueString()
		putStr(m, "token", a.Token)
	case "environment_variable":
		m["secret_name"] = a.SecretName.ValueString()
		putStr(m, "secret_value", a.SecretValue)
		if a.Networking != nil {
			nw, d := buildNetworking(ctx, a.Networking)
			diags.Append(d...)
			m["networking"] = nw
		}
		if il := buildInjection(a.InjectionLocation); il != nil {
			m["injection_location"] = il
		}
	}

	return marshalAuth(m, &diags)
}

// buildAuthUpdate builds the mutable subset of the auth union for an update.
// Structural keys (mcp_server_url, secret_name, refresh.token_endpoint,
// refresh.client_id) are immutable and are omitted.
func buildAuthUpdate(ctx context.Context, a *credAuthBlock) (json.RawMessage, diag.Diagnostics) {
	var diags diag.Diagnostics
	if a == nil {
		return nil, diags
	}
	m := map[string]interface{}{"type": a.Type.ValueString()}

	switch a.Type.ValueString() {
	case "mcp_oauth":
		putStr(m, "access_token", a.AccessToken)
		putStr(m, "expires_at", a.ExpiresAt)
		if a.Refresh != nil {
			rf := map[string]interface{}{}
			putStr(rf, "refresh_token", a.Refresh.RefreshToken)
			putStr(rf, "scope", a.Refresh.Scope)
			if tea := a.Refresh.TokenEndpointAuth; tea != nil && !tea.Type.IsNull() && !tea.Type.IsUnknown() {
				teaM := map[string]interface{}{"type": tea.Type.ValueString()}
				putStr(teaM, "client_secret", tea.ClientSecret)
				rf["token_endpoint_auth"] = teaM
			}
			if len(rf) > 0 {
				m["refresh"] = rf
			}
		}
	case "static_bearer":
		putStr(m, "token", a.Token)
	case "environment_variable":
		putStr(m, "secret_value", a.SecretValue)
		if a.Networking != nil {
			nw, d := buildNetworking(ctx, a.Networking)
			diags.Append(d...)
			m["networking"] = nw
		}
		if il := buildInjection(a.InjectionLocation); il != nil {
			m["injection_location"] = il
		}
	}

	return marshalAuth(m, &diags)
}

func buildNetworking(ctx context.Context, n *credNetworkingBlock) (map[string]interface{}, diag.Diagnostics) {
	m := map[string]interface{}{"type": n.Type.ValueString()}
	hosts, diags := listToStrings(ctx, n.AllowedHosts)
	if len(hosts) > 0 {
		m["allowed_hosts"] = hosts
	}
	return m, diags
}

// buildInjection returns the injection_location payload with only the fields
// the user set (each omitted field defaults to false on create, or is left
// unchanged on update). Returns nil when nothing is set.
func buildInjection(il *credInjectionBlock) map[string]interface{} {
	if il == nil {
		return nil
	}
	m := map[string]interface{}{}
	if !il.Header.IsNull() && !il.Header.IsUnknown() {
		m["header"] = il.Header.ValueBool()
	}
	if !il.Body.IsNull() && !il.Body.IsUnknown() {
		m["body"] = il.Body.ValueBool()
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func putStr(m map[string]interface{}, key string, v types.String) {
	if p := stringPtr(v); p != nil {
		m[key] = *p
	}
}

func marshalAuth(m map[string]interface{}, diags *diag.Diagnostics) (json.RawMessage, diag.Diagnostics) {
	b, err := json.Marshal(m)
	if err != nil {
		diags.AddError("Error encoding auth", err.Error())
	}
	return b, *diags
}

// credentialFromAPI maps an API credential into the resource model, leaving all
// write-only secret fields null (they are never returned and must not be
// persisted in state). secretVersion and the refresh token_endpoint_auth block
// are carried from prior (plan on create/update, state on read), since the API
// echoes only the auth type there — nothing worth drift-detecting — and its
// write-only client_secret can never be refreshed.
func credentialFromAPI(ctx context.Context, c *client.Credential, prior *credAuthBlock, secretVersion types.String, diags *diag.Diagnostics) credentialResourceModel {
	m := credentialResourceModel{
		ID:            types.StringValue(c.ID),
		Type:          types.StringValue(c.Type),
		VaultID:       types.StringValue(c.VaultID),
		DisplayName:   stringFromPtr(c.DisplayName),
		SecretVersion: secretVersion,
		CreatedAt:     types.StringValue(c.CreatedAt),
		UpdatedAt:     types.StringValue(c.UpdatedAt),
		ArchivedAt:    stringFromPtr(c.ArchivedAt),
	}

	if len(c.Metadata) == 0 {
		m.Metadata = types.MapNull(types.StringType)
	} else {
		mv, d := stringMapToMap(ctx, c.Metadata)
		diags.Append(d...)
		m.Metadata = mv
	}

	a := &credAuthBlock{
		Type:         types.StringValue(c.Auth.Type),
		MCPServerURL: types.StringNull(),
		AccessToken:  types.StringNull(),
		ExpiresAt:    types.StringNull(),
		Token:        types.StringNull(),
		SecretName:   types.StringNull(),
		SecretValue:  types.StringNull(),
	}

	switch c.Auth.Type {
	case "mcp_oauth":
		a.MCPServerURL = types.StringValue(c.Auth.MCPServerURL)
		a.ExpiresAt = stringFromPtr(c.Auth.ExpiresAt)
		if rf := c.Auth.Refresh; rf != nil {
			a.Refresh = &credRefreshBlock{
				TokenEndpoint:     types.StringValue(rf.TokenEndpoint),
				ClientID:          types.StringValue(rf.ClientID),
				Scope:             stringFromPtr(rf.Scope),
				Resource:          stringFromPtr(rf.Resource),
				RefreshToken:      types.StringNull(),
				TokenEndpointAuth: priorTokenEndpointAuth(prior),
			}
		}
	case "static_bearer":
		a.MCPServerURL = types.StringValue(c.Auth.MCPServerURL)
	case "environment_variable":
		a.SecretName = types.StringValue(c.Auth.SecretName)
		if nw := c.Auth.Networking; nw != nil {
			hosts, d := stringsToList(ctx, nw.AllowedHosts)
			diags.Append(d...)
			a.Networking = &credNetworkingBlock{Type: types.StringValue(nw.Type), AllowedHosts: hosts}
		}
		if il := c.Auth.InjectionLocation; il != nil {
			a.InjectionLocation = &credInjectionBlock{
				Header: types.BoolValue(il.Header),
				Body:   types.BoolValue(il.Body),
			}
		}
	}

	m.Auth = a
	return m
}

// priorTokenEndpointAuth carries the configured token_endpoint_auth block
// (with its write-only client_secret already null in plan/state) forward, so
// state matches config. Returns nil when it was not configured.
func priorTokenEndpointAuth(prior *credAuthBlock) *credTokenEndpointAuthBlock {
	if prior == nil || prior.Refresh == nil || prior.Refresh.TokenEndpointAuth == nil {
		return nil
	}
	src := prior.Refresh.TokenEndpointAuth
	return &credTokenEndpointAuthBlock{
		Type:         src.Type,
		ClientSecret: types.StringNull(),
	}
}

func splitTwo(s, sep string) (string, string, bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+len(sep):], true
}
