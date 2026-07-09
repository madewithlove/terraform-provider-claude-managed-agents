package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/jsontype"
)

var (
	_ resource.Resource                = (*agentResource)(nil)
	_ resource.ResourceWithConfigure   = (*agentResource)(nil)
	_ resource.ResourceWithImportState = (*agentResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*agentResource)(nil)
)

type agentResource struct {
	client *client.Client
}

// NewAgentResource is the resource factory.
func NewAgentResource() resource.Resource { return &agentResource{} }

type agentModelBlock struct {
	ID    types.String `tfsdk:"id"`
	Speed types.String `tfsdk:"speed"`
}

type agentResourceModel struct {
	ID          types.String     `tfsdk:"id"`
	Type        types.String     `tfsdk:"type"`
	Name        types.String     `tfsdk:"name"`
	Model       *agentModelBlock `tfsdk:"model"`
	System      types.String     `tfsdk:"system"`
	Description types.String     `tfsdk:"description"`
	Tools       jsontype.Subset  `tfsdk:"tools"`
	Skills      jsontype.Subset  `tfsdk:"skills"`
	MCPServers  jsontype.Subset  `tfsdk:"mcp_servers"`
	Multiagent  jsontype.Subset  `tfsdk:"multiagent"`
	Metadata    types.Map        `tfsdk:"metadata"`
	Version     types.Int64      `tfsdk:"version"`
	CreatedAt   types.String     `tfsdk:"created_at"`
	UpdatedAt   types.String     `tfsdk:"updated_at"`
	ArchivedAt  types.String     `tfsdk:"archived_at"`
}

func (r *agentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent"
}

func (r *agentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A reusable, versioned Claude agent configuration (model, system prompt, tools, MCP servers, and skills). " +
			"Agents cannot be hard-deleted; `terraform destroy` archives the agent (existing sessions keep running).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Agent identifier.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Object type (always `agent`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable name for the agent.",
			},
			"model": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "The Claude model that powers the agent.",
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Model ID, e.g. `claude-opus-4-8`. Claude 4.5-family and later are supported.",
					},
					"speed": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Inference speed: `standard` (default) or `fast`. Fast mode requires a supported Opus model.",
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
				},
			},
			"system": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "System prompt defining the agent's behavior and persona.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Description of what the agent does.",
			},
			"tools": schema.StringAttribute{
				CustomType:    jsontype.SubsetType{},
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), suppressJSONSubset()},
				MarkdownDescription: "Tools available to the agent, as a JSON array. Combines pre-built agent tools, MCP tools, and custom tools. " +
					"Refreshed on read. The API enriches each entry with defaults (e.g. `default_config`); a config value that is a recursive subset of the enriched server value is treated as unchanged, so this plans cleanly on import and never churns.",
			},
			"mcp_servers": schema.StringAttribute{
				CustomType:          jsontype.SubsetType{},
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown(), suppressJSONSubset()},
				MarkdownDescription: "MCP servers, as a JSON array. Refreshed on read; server-enriched fields are tolerated (subset semantics).",
			},
			"skills": schema.StringAttribute{
				CustomType:          jsontype.SubsetType{},
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown(), suppressJSONSubset()},
				MarkdownDescription: "Skills, as a JSON array. Refreshed on read; server-enriched fields are tolerated (subset semantics).",
			},
			"multiagent": schema.StringAttribute{
				CustomType:          jsontype.SubsetType{},
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown(), suppressJSONSubset()},
				MarkdownDescription: "Coordinator declaration listing agents this agent can delegate to, as a JSON object. Refreshed on read; server-enriched fields are tolerated (subset semantics).",
			},
			"metadata": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Arbitrary key-value pairs for your own tracking.",
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Current agent version. Starts at 1 and increments on each change.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp (RFC 3339).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last-update timestamp (RFC 3339).",
			},
			"archived_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Archive timestamp (RFC 3339), or null if active.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *agentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *agentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan agentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metadata, diags := mapToStringMap(ctx, plan.Metadata)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := client.AgentCreateRequest{
		Name:        plan.Name.ValueString(),
		Model:       modelConfigFromBlock(plan.Model),
		System:      stringPtr(plan.System),
		Description: stringPtr(plan.Description),
		Tools:       rawFromSubset(plan.Tools),
		Skills:      rawFromSubset(plan.Skills),
		MCPServers:  rawFromSubset(plan.MCPServers),
		Multiagent:  rawFromSubset(plan.Multiagent),
		Metadata:    metadata,
	}

	agent, err := r.client.CreateAgent(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating agent", err.Error())
		return
	}

	// tools/skills/mcp_servers/multiagent are Optional+Computed, so populate
	// them from the (enriched) response. The create-time semantic equality
	// then keeps the planned subset value in state, so state stays == config.
	plan.Tools = subsetFromRaw(agent.Tools)
	plan.Skills = subsetFromRaw(agent.Skills)
	plan.MCPServers = subsetFromRaw(agent.MCPServers)
	plan.Multiagent = subsetFromRaw(agent.Multiagent)
	r.applyComputed(&plan, agent)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *agentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state agentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agent, err := r.client.GetAgent(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}

	// Refresh all drift-detectable fields, including the JSON fields. The API
	// enriches tools/skills/mcp_servers/multiagent on the way back; the Subset
	// custom type's semantic equality keeps a managed subset value stable, and
	// the subset plan modifier keeps imported (enriched) values planning clean.
	state.Name = types.StringValue(agent.Name)
	state.System = stringFromPtr(agent.System)
	state.Description = stringFromPtr(agent.Description)
	state.Tools = subsetFromRaw(agent.Tools)
	state.Skills = subsetFromRaw(agent.Skills)
	state.MCPServers = subsetFromRaw(agent.MCPServers)
	state.Multiagent = subsetFromRaw(agent.Multiagent)
	r.applyModel(&state, agent)
	r.applyMetadata(ctx, &state, agent, &resp.Diagnostics)
	r.applyComputed(&state, agent)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *agentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config agentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	// The subset plan modifier may rewrite the planned JSON fields to the
	// enriched prior-state value to suppress the diff. Source the request
	// payload from config so we always send exactly what the user declared.
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Compute metadata with removed keys nulled out for declarative behavior.
	metadata, diags := mergedMetadata(ctx, state.Metadata, plan.Metadata)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.AgentUpdateRequest{
		Version:     state.Version.ValueInt64(),
		Name:        plan.Name.ValueString(),
		Model:       modelConfigFromBlock(plan.Model),
		System:      stringPtr(plan.System),
		Description: stringPtr(plan.Description),
		Tools:       rawFromSubset(config.Tools),
		Skills:      rawFromSubset(config.Skills),
		MCPServers:  rawFromSubset(config.MCPServers),
		Multiagent:  rawFromSubset(config.Multiagent),
		Metadata:    metadata,
	}

	agent, err := r.client.UpdateAgent(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		if client.IsConflict(err) {
			resp.Diagnostics.AddError(
				"Agent version conflict",
				"The agent was modified outside Terraform (version mismatch). Run `terraform refresh` or `terraform apply -refresh-only` and try again.\n\n"+err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError("Error updating agent", err.Error())
		return
	}

	r.applyComputed(&plan, agent)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *agentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state agentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.ArchiveAgent(ctx, state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error archiving agent", err.Error())
		return
	}
	resp.Diagnostics.AddWarning(
		"Agent archived, not deleted",
		"Claude agents cannot be hard-deleted. The agent has been archived and is now read-only; existing sessions continue to run.",
	)
}

func (r *agentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan prevents a phantom in-place update on refresh/import. After the
// attribute plan modifiers run (subset suppression on the JSON fields), the
// only remaining "changes" can be the server-computed churn fields `version`
// and `updated_at`, which the framework marks unknown as soon as the resource
// is flagged for update. When every configurable and refreshable attribute
// already matches state, we pin those two to their prior values so the plan is
// a true no-op. A genuine change leaves them unknown so the real update (and
// version bump) proceeds correctly.
func (r *agentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return // create or destroy
	}

	var plan, state agentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Tentatively pin the server-computed churn fields to prior state.
	plan.Version = state.Version
	plan.UpdatedAt = state.UpdatedAt

	// If the plan now matches state in every field, it is a no-op; commit the
	// pinned values so the plan is empty. Otherwise leave the plan untouched
	// (the framework's unknown version/updated_at drive the real update).
	if agentModelsEqual(plan, state) {
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	}
}

// agentModelsEqual reports whether two agent models are equal across every
// attribute. The JSON fields compare byte-equal here because the subset plan
// modifier has already aligned the plan value to the prior state value when
// the config was a subset.
func agentModelsEqual(a, b agentResourceModel) bool {
	if !a.ID.Equal(b.ID) || !a.Type.Equal(b.Type) || !a.Name.Equal(b.Name) ||
		!a.System.Equal(b.System) || !a.Description.Equal(b.Description) ||
		!a.Tools.Equal(b.Tools) || !a.Skills.Equal(b.Skills) ||
		!a.MCPServers.Equal(b.MCPServers) || !a.Multiagent.Equal(b.Multiagent) ||
		!a.Metadata.Equal(b.Metadata) || !a.Version.Equal(b.Version) ||
		!a.CreatedAt.Equal(b.CreatedAt) || !a.UpdatedAt.Equal(b.UpdatedAt) ||
		!a.ArchivedAt.Equal(b.ArchivedAt) {
		return false
	}
	switch {
	case a.Model == nil && b.Model == nil:
		return true
	case a.Model == nil || b.Model == nil:
		return false
	default:
		return a.Model.ID.Equal(b.Model.ID) && a.Model.Speed.Equal(b.Model.Speed)
	}
}

// applyComputed sets the server-owned computed fields.
func (r *agentResource) applyComputed(m *agentResourceModel, a *client.Agent) {
	m.ID = types.StringValue(a.ID)
	m.Type = types.StringValue(a.Type)
	m.Version = types.Int64Value(a.Version)
	m.CreatedAt = types.StringValue(a.CreatedAt)
	m.UpdatedAt = types.StringValue(a.UpdatedAt)
	m.ArchivedAt = stringFromPtr(a.ArchivedAt)
	// model may carry a server-defaulted speed; keep it in sync.
	r.applyModel(m, a)
}

func (r *agentResource) applyModel(m *agentResourceModel, a *client.Agent) {
	m.Model = &agentModelBlock{
		ID:    types.StringValue(a.Model.ID),
		Speed: types.StringValue(a.Model.Speed),
	}
}

func (r *agentResource) applyMetadata(ctx context.Context, m *agentResourceModel, a *client.Agent, diags *diag.Diagnostics) {
	// Treat an empty server map as null only when the config had none, to keep
	// plans stable.
	if len(a.Metadata) == 0 {
		m.Metadata = types.MapNull(types.StringType)
		return
	}
	mv, d := stringMapToMap(ctx, a.Metadata)
	diags.Append(d...)
	m.Metadata = mv
}

func modelConfigFromBlock(b *agentModelBlock) client.ModelConfig {
	if b == nil {
		return client.ModelConfig{}
	}
	mc := client.ModelConfig{ID: b.ID.ValueString()}
	if !b.Speed.IsNull() && !b.Speed.IsUnknown() {
		mc.Speed = b.Speed.ValueString()
	}
	return mc
}
