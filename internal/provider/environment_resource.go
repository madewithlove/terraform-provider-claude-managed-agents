package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ resource.Resource                = (*environmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*environmentResource)(nil)
	_ resource.ResourceWithImportState = (*environmentResource)(nil)
)

type environmentResource struct {
	client *client.Client
}

// NewEnvironmentResource is the resource factory.
func NewEnvironmentResource() resource.Resource { return &environmentResource{} }

type envPackagesBlock struct {
	Apt   types.List `tfsdk:"apt"`
	Cargo types.List `tfsdk:"cargo"`
	Gem   types.List `tfsdk:"gem"`
	Go    types.List `tfsdk:"go"`
	Npm   types.List `tfsdk:"npm"`
	Pip   types.List `tfsdk:"pip"`
}

type envNetworkingBlock struct {
	Type                 types.String `tfsdk:"type"`
	AllowedHosts         types.List   `tfsdk:"allowed_hosts"`
	AllowMCPServers      types.Bool   `tfsdk:"allow_mcp_servers"`
	AllowPackageManagers types.Bool   `tfsdk:"allow_package_managers"`
}

type envConfigBlock struct {
	Type       types.String        `tfsdk:"type"`
	Packages   *envPackagesBlock   `tfsdk:"packages"`
	Networking *envNetworkingBlock `tfsdk:"networking"`
}

type environmentResourceModel struct {
	ID         types.String    `tfsdk:"id"`
	Type       types.String    `tfsdk:"type"`
	Name       types.String    `tfsdk:"name"`
	Config     *envConfigBlock `tfsdk:"config"`
	CreatedAt  types.String    `tfsdk:"created_at"`
	UpdatedAt  types.String    `tfsdk:"updated_at"`
	ArchivedAt types.String    `tfsdk:"archived_at"`
}

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	stringListAttr := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{ElementType: types.StringType, Optional: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Claude Managed Agents environment: the sandbox configuration sessions run in. " +
			"Environments are not versioned or updatable, so any change forces replacement. `config` is not refreshed on read.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique, descriptive environment name.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"config": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "Sandbox configuration. Changing any part forces replacement.",
				PlanModifiers:       []planmodifier.Object{objectplanmodifier.RequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "`cloud` (Anthropic-managed sandbox) or `self_hosted`.",
					},
					"packages": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "Packages to pre-install (cloud only), keyed by package manager. Versions may be pinned per that manager's syntax.",
						Attributes: map[string]schema.Attribute{
							"apt":   stringListAttr("System packages (apt-get)."),
							"cargo": stringListAttr("Rust crates (cargo)."),
							"gem":   stringListAttr("Ruby gems."),
							"go":    stringListAttr("Go modules."),
							"npm":   stringListAttr("Node.js packages (npm)."),
							"pip":   stringListAttr("Python packages (pip)."),
						},
					},
					"networking": schema.SingleNestedAttribute{
						Optional:            true,
						MarkdownDescription: "Outbound network access for the sandbox (cloud only).",
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "`unrestricted` (default when networking is omitted) or `limited`.",
							},
							"allowed_hosts": stringListAttr("Hostnames or wildcard patterns the sandbox may reach (`limited` only). No scheme, port, or path."),
							"allow_mcp_servers": schema.BoolAttribute{
								Optional:            true,
								MarkdownDescription: "Allow outbound access to the agent's configured MCP servers (`limited` only). Defaults to false.",
							},
							"allow_package_managers": schema.BoolAttribute{
								Optional:            true,
								MarkdownDescription: "Allow outbound access to public package registries (`limited` only). Defaults to false.",
							},
						},
					},
				},
			},
			"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp (RFC 3339)."},
			"updated_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Last-update timestamp (RFC 3339)."},
			"archived_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Archive timestamp (RFC 3339), or null if active."},
		},
	}
}

func (r *environmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, diags := envConfigToAPI(ctx, plan.Config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	env, err := r.client.CreateEnvironment(ctx, client.EnvironmentCreateRequest{
		Name:   plan.Name.ValueString(),
		Config: cfg,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating environment", err.Error())
		return
	}

	applyEnvComputed(&plan, env)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	env, err := r.client.GetEnvironment(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading environment", err.Error())
		return
	}

	// config is intentionally not refreshed (immutable + server enrichment).
	state.Name = types.StringValue(env.Name)
	applyEnvComputed(&state, env)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every configurable attribute is RequiresReplace, so
// Terraform destroys and recreates instead of updating. It exists to satisfy
// the resource interface.
func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Environment update not supported",
		"Environments are immutable; changes should force replacement. This is a provider bug if you see it.",
	)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	err := r.client.DeleteEnvironment(ctx, id)
	if err == nil || client.IsNotFound(err) {
		return
	}

	// The API refuses to delete an environment still referenced by a session.
	// Fall back to archiving so destroy can complete.
	if client.IsConflict(err) {
		if _, aerr := r.client.ArchiveEnvironment(ctx, id); aerr != nil {
			resp.Diagnostics.AddError("Error archiving environment", aerr.Error())
			return
		}
		resp.Diagnostics.AddWarning(
			"Environment archived instead of deleted",
			"The environment could not be deleted because one or more sessions still reference it, so it was archived (read-only) instead.",
		)
		return
	}
	resp.Diagnostics.AddError("Error deleting environment", err.Error())
}

func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func applyEnvComputed(m *environmentResourceModel, e *client.Environment) {
	m.ID = types.StringValue(e.ID)
	m.Type = types.StringValue(e.Type)
	m.CreatedAt = types.StringValue(e.CreatedAt)
	m.UpdatedAt = types.StringValue(e.UpdatedAt)
	m.ArchivedAt = stringFromPtr(e.ArchivedAt)
}

func envConfigToAPI(ctx context.Context, b *envConfigBlock) (client.EnvironmentConfig, diag.Diagnostics) {
	var diags diag.Diagnostics
	cfg := client.EnvironmentConfig{Type: b.Type.ValueString()}

	if b.Packages != nil {
		p := &client.Packages{}
		for _, m := range []struct {
			src types.List
			dst *[]string
		}{
			{b.Packages.Apt, &p.Apt},
			{b.Packages.Cargo, &p.Cargo},
			{b.Packages.Gem, &p.Gem},
			{b.Packages.Go, &p.Go},
			{b.Packages.Npm, &p.Npm},
			{b.Packages.Pip, &p.Pip},
		} {
			vals, d := listToStrings(ctx, m.src)
			diags.Append(d...)
			*m.dst = vals
		}
		cfg.Packages = p
	}

	if b.Networking != nil {
		n := &client.Networking{Type: b.Networking.Type.ValueString()}
		hosts, d := listToStrings(ctx, b.Networking.AllowedHosts)
		diags.Append(d...)
		n.AllowedHosts = hosts
		if !b.Networking.AllowMCPServers.IsNull() && !b.Networking.AllowMCPServers.IsUnknown() {
			v := b.Networking.AllowMCPServers.ValueBool()
			n.AllowMCPServers = &v
		}
		if !b.Networking.AllowPackageManagers.IsNull() && !b.Networking.AllowPackageManagers.IsUnknown() {
			v := b.Networking.AllowPackageManagers.ValueBool()
			n.AllowPackageManagers = &v
		}
		cfg.Networking = n
	}

	return cfg, diags
}
