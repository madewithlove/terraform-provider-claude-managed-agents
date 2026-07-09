package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ resource.Resource                   = (*credentialResource)(nil)
	_ resource.ResourceWithConfigure      = (*credentialResource)(nil)
	_ resource.ResourceWithImportState    = (*credentialResource)(nil)
	_ resource.ResourceWithValidateConfig = (*credentialResource)(nil)
)

type credentialResource struct {
	client *client.Client
}

// NewVaultCredentialResource is the resource factory.
func NewVaultCredentialResource() resource.Resource { return &credentialResource{} }

type credTokenEndpointAuthBlock struct {
	Type         types.String `tfsdk:"type"`
	ClientSecret types.String `tfsdk:"client_secret"` // write-only
}

type credRefreshBlock struct {
	TokenEndpoint     types.String                `tfsdk:"token_endpoint"` // immutable
	ClientID          types.String                `tfsdk:"client_id"`      // immutable
	Scope             types.String                `tfsdk:"scope"`
	Resource          types.String                `tfsdk:"resource"`      // response-only
	RefreshToken      types.String                `tfsdk:"refresh_token"` // write-only
	TokenEndpointAuth *credTokenEndpointAuthBlock `tfsdk:"token_endpoint_auth"`
}

type credNetworkingBlock struct {
	Type         types.String `tfsdk:"type"`
	AllowedHosts types.List   `tfsdk:"allowed_hosts"`
}

type credInjectionBlock struct {
	Header types.Bool `tfsdk:"header"`
	Body   types.Bool `tfsdk:"body"`
}

type credAuthBlock struct {
	Type              types.String         `tfsdk:"type"`           // immutable
	MCPServerURL      types.String         `tfsdk:"mcp_server_url"` // immutable
	AccessToken       types.String         `tfsdk:"access_token"`   // write-only
	ExpiresAt         types.String         `tfsdk:"expires_at"`
	Token             types.String         `tfsdk:"token"`        // write-only
	SecretName        types.String         `tfsdk:"secret_name"`  // immutable
	SecretValue       types.String         `tfsdk:"secret_value"` // write-only
	Refresh           *credRefreshBlock    `tfsdk:"refresh"`
	Networking        *credNetworkingBlock `tfsdk:"networking"`
	InjectionLocation *credInjectionBlock  `tfsdk:"injection_location"`
}

type credentialResourceModel struct {
	ID            types.String   `tfsdk:"id"`
	Type          types.String   `tfsdk:"type"`
	VaultID       types.String   `tfsdk:"vault_id"` // immutable (parent)
	DisplayName   types.String   `tfsdk:"display_name"`
	Metadata      types.Map      `tfsdk:"metadata"`
	SecretVersion types.String   `tfsdk:"secret_version"`
	Auth          *credAuthBlock `tfsdk:"auth"`
	CreatedAt     types.String   `tfsdk:"created_at"`
	UpdatedAt     types.String   `tfsdk:"updated_at"`
	ArchivedAt    types.String   `tfsdk:"archived_at"`
}

func (r *credentialResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_credential"
}

func (r *credentialResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "A credential stored in a vault. Supports MCP OAuth, MCP static bearer, and environment-variable auth. " +
			"Secret values are **write-only**: they are sent to the API but never stored in Terraform state, and never returned by the API. " +
			"Structural keys (`type`, `mcp_server_url`, `secret_name`, `refresh.token_endpoint`, `refresh.client_id`) are immutable and force replacement. " +
			"`terraform destroy` hard-deletes the credential.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vault_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the vault this credential belongs to.",
				PlanModifiers:       replaceStr,
			},
			"display_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Human-readable name for the credential (1-255 characters).",
			},
			"metadata": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Arbitrary key-value pairs.",
			},
			"secret_version": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Change this value to force the provider to re-send the write-only secret(s) from configuration " +
					"(e.g. to rotate a token). Changes to write-only values alone are invisible to Terraform, so bump this to apply them.",
			},
			"auth": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Authentication details.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "`mcp_oauth`, `static_bearer`, or `environment_variable`.",
						PlanModifiers:       replaceStr,
						Validators: []validator.String{
							stringvalidator.OneOf("mcp_oauth", "static_bearer", "environment_variable"),
						},
					},
					"mcp_server_url": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "MCP server URL this credential authenticates against (`mcp_oauth`, `static_bearer`). Immutable.",
						PlanModifiers:       replaceStr,
					},
					"access_token": schema.StringAttribute{
						Optional:            true,
						WriteOnly:           true,
						Sensitive:           true,
						MarkdownDescription: "OAuth access token (`mcp_oauth`). Write-only.",
					},
					"expires_at": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Access-token expiry (RFC 3339) for `mcp_oauth`.",
					},
					"token": schema.StringAttribute{
						Optional:            true,
						WriteOnly:           true,
						Sensitive:           true,
						MarkdownDescription: "Static bearer token (`static_bearer`). Write-only.",
					},
					"secret_name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Environment variable name (`environment_variable`). Immutable.",
						PlanModifiers:       replaceStr,
					},
					"secret_value": schema.StringAttribute{
						Optional:            true,
						WriteOnly:           true,
						Sensitive:           true,
						MarkdownDescription: "Secret value (`environment_variable`). Write-only.",
					},
					"refresh": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "OAuth refresh configuration (`mcp_oauth`). Anthropic refreshes the access token on your behalf.",
						Attributes: map[string]schema.Attribute{
							"token_endpoint": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "Token endpoint URL. Immutable.",
								PlanModifiers:       replaceStr,
							},
							"client_id": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "OAuth client ID. Immutable.",
								PlanModifiers:       replaceStr,
							},
							"scope": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "OAuth scope for the refresh request.",
							},
							"resource": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "OAuth resource indicator (response-only).",
							},
							"refresh_token": schema.StringAttribute{
								Optional:            true,
								WriteOnly:           true,
								Sensitive:           true,
								MarkdownDescription: "OAuth refresh token. Write-only.",
							},
							"token_endpoint_auth": schema.SingleNestedAttribute{
								Optional:            true,
								MarkdownDescription: "How the refresh call authenticates. Omit for a public client (`none`).",
								Attributes: map[string]schema.Attribute{
									"type": schema.StringAttribute{
										Required:            true,
										MarkdownDescription: "`none`, `client_secret_basic`, or `client_secret_post`.",
										Validators: []validator.String{
											stringvalidator.OneOf("none", "client_secret_basic", "client_secret_post"),
										},
									},
									"client_secret": schema.StringAttribute{
										Optional:            true,
										WriteOnly:           true,
										Sensitive:           true,
										MarkdownDescription: "OAuth client secret. Write-only.",
									},
								},
							},
						},
					},
					"networking": schema.SingleNestedAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Which outbound hosts the secret is substituted on (`environment_variable`).",
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "`unrestricted` or `limited`.",
								Validators: []validator.String{
									stringvalidator.OneOf("unrestricted", "limited"),
								},
							},
							"allowed_hosts": schema.ListAttribute{
								ElementType:         types.StringType,
								Optional:            true,
								Computed:            true,
								MarkdownDescription: "Hosts the secret may be substituted on (`limited`; max 16). Bare hostnames, IPv4, or `*.`-wildcards.",
							},
						},
					},
					"injection_location": schema.SingleNestedAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Which parts of an outbound request the secret is substituted into (`environment_variable`). Both default to true when omitted.",
						Attributes: map[string]schema.Attribute{
							"header": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Substitute in request headers."},
							"body":   schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Substitute in the request body."},
						},
					},
				},
			},
			"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
			"updated_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Last-update timestamp (RFC 3339)."},
			"archived_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Archive timestamp (RFC 3339), or null."},
		},
	}
}

func (r *credentialResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg credentialResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.Auth == nil || cfg.Auth.Type.IsUnknown() {
		return
	}

	authPath := path.Root("auth")
	switch cfg.Auth.Type.ValueString() {
	case "mcp_oauth", "static_bearer":
		if cfg.Auth.MCPServerURL.IsNull() {
			resp.Diagnostics.AddAttributeError(authPath.AtName("mcp_server_url"), "Missing mcp_server_url",
				fmt.Sprintf("`auth.mcp_server_url` is required when auth.type is %q.", cfg.Auth.Type.ValueString()))
		}
		if cfg.Auth.SecretName.ValueString() != "" {
			resp.Diagnostics.AddAttributeError(authPath.AtName("secret_name"), "Unexpected secret_name",
				"`auth.secret_name` only applies to environment_variable credentials.")
		}
	case "environment_variable":
		if cfg.Auth.SecretName.IsNull() {
			resp.Diagnostics.AddAttributeError(authPath.AtName("secret_name"), "Missing secret_name",
				"`auth.secret_name` is required when auth.type is \"environment_variable\".")
		}
		if cfg.Auth.MCPServerURL.ValueString() != "" {
			resp.Diagnostics.AddAttributeError(authPath.AtName("mcp_server_url"), "Unexpected mcp_server_url",
				"`auth.mcp_server_url` only applies to MCP credentials.")
		}
		if cfg.Auth.Refresh != nil {
			resp.Diagnostics.AddAttributeError(authPath.AtName("refresh"), "Unexpected refresh",
				"`auth.refresh` only applies to mcp_oauth credentials.")
		}
	}
}

func (r *credentialResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *credentialResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan credentialResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// Secrets live only in config (write-only values are absent from the plan).
	var cfg credentialResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metadata, diags := mapToStringMap(ctx, plan.Metadata)
	resp.Diagnostics.Append(diags...)
	auth, diags := buildAuthCreate(ctx, cfg.Auth)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cred, err := r.client.CreateCredential(ctx, plan.VaultID.ValueString(), client.CredentialCreateRequest{
		DisplayName: stringPtr(plan.DisplayName),
		Metadata:    metadata,
		Auth:        auth,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating credential", err.Error())
		return
	}

	state := credentialFromAPI(ctx, cred, plan.Auth, plan.SecretVersion, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *credentialResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state credentialResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cred, err := r.client.GetCredential(ctx, state.VaultID.ValueString(), state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading credential", err.Error())
		return
	}

	next := credentialFromAPI(ctx, cred, state.Auth, state.SecretVersion, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *credentialResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan credentialResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var cfg credentialResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	var state credentialResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metadata, diags := mergedMetadata(ctx, state.Metadata, plan.Metadata)
	resp.Diagnostics.Append(diags...)
	auth, diags := buildAuthUpdate(ctx, cfg.Auth)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cred, err := r.client.UpdateCredential(ctx, state.VaultID.ValueString(), state.ID.ValueString(), client.CredentialUpdateRequest{
		DisplayName: stringPtr(plan.DisplayName),
		Metadata:    metadata,
		Auth:        auth,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating credential", err.Error())
		return
	}

	next := credentialFromAPI(ctx, cred, cfg.Auth, plan.SecretVersion, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *credentialResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state credentialResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCredential(ctx, state.VaultID.ValueString(), state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting credential", err.Error())
	}
}

// ImportState expects "<vault_id>/<credential_id>".
func (r *credentialResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vaultID, credID, ok := splitTwo(req.ID, "/")
	if !ok {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"<vault_id>/<credential_id>\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vault_id"), vaultID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), credID)...)
}
