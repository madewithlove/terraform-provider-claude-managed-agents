package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ resource.Resource                = (*deploymentResource)(nil)
	_ resource.ResourceWithConfigure   = (*deploymentResource)(nil)
	_ resource.ResourceWithImportState = (*deploymentResource)(nil)
)

type deploymentResource struct {
	client *client.Client
}

// NewDeploymentResource is the resource factory.
func NewDeploymentResource() resource.Resource { return &deploymentResource{} }

type deploymentScheduleBlock struct {
	Type           types.String `tfsdk:"type"`
	Expression     types.String `tfsdk:"expression"`
	Timezone       types.String `tfsdk:"timezone"`
	LastRunAt      types.String `tfsdk:"last_run_at"`
	UpcomingRunsAt types.List   `tfsdk:"upcoming_runs_at"`
}

type deploymentResourceModel struct {
	ID            types.String             `tfsdk:"id"`
	Type          types.String             `tfsdk:"type"`
	Name          types.String             `tfsdk:"name"`
	AgentID       types.String             `tfsdk:"agent_id"`
	EnvironmentID types.String             `tfsdk:"environment_id"`
	InitialEvents jsontypes.Normalized     `tfsdk:"initial_events"`
	Schedule      *deploymentScheduleBlock `tfsdk:"schedule"`
	Files         jsontypes.Normalized     `tfsdk:"files"`
	GitHub        jsontypes.Normalized     `tfsdk:"github"`
	MemoryStores  jsontypes.Normalized     `tfsdk:"memory_stores"`
	Vaults        jsontypes.Normalized     `tfsdk:"vaults"`
	Paused        types.Bool               `tfsdk:"paused"`
	Status        types.String             `tfsdk:"status"`
	PausedReason  jsontypes.Normalized     `tfsdk:"paused_reason"`
	CreatedAt     types.String             `tfsdk:"created_at"`
}

func (r *deploymentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (r *deploymentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	// replaceIfChanged forces replacement only when the prior state is known and
	// changes. On import the prior is null (these fields cannot be refreshed
	// from the API), so the config value is adopted rather than triggering a
	// destroy/recreate.
	replaceIfChanged := []planmodifier.String{requiresReplaceIfKnownChanged()}
	replaceJSON := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			CustomType:          jsontypes.NormalizedType{},
			Optional:            true,
			MarkdownDescription: desc,
			PlanModifiers:       replaceIfChanged,
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A scheduled deployment: runs an agent on a recurring cron schedule. " +
			"Deployment configuration is immutable — every field except `paused` forces replacement. " +
			"`terraform destroy` archives the deployment (terminal).",
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
				MarkdownDescription: "Human-readable deployment name.",
				PlanModifiers:       replaceStr,
			},
			"agent_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the agent to run. Refreshed on read (from the returned agent object); a change forces replacement.",
				PlanModifiers:       replaceIfChanged,
			},
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the environment to run in. Refreshed on read; a change forces replacement.",
				PlanModifiers:       replaceIfChanged,
			},
			"initial_events": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				Required:            true,
				MarkdownDescription: "JSON array of events that start each run. Must include an initial `user.message`. Not returned by the API, so it is adopted from config on import and a change forces replacement.",
				PlanModifiers:       replaceIfChanged,
			},
			"schedule": schema.SingleNestedAttribute{
				Required:            true,
				MarkdownDescription: "The recurring cron schedule.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						MarkdownDescription: "Schedule type. Defaults to `cron`.",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							stringplanmodifier.RequiresReplace(),
						},
					},
					"expression": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "POSIX cron expression (`minute hour day-of-month month day-of-week`).",
						PlanModifiers:       replaceStr,
					},
					"timezone": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "IANA timezone identifier, e.g. `America/New_York`.",
						PlanModifiers:       replaceStr,
					},
					"last_run_at": schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Timestamp of the last run, or null.",
					},
					"upcoming_runs_at": schema.ListAttribute{
						ElementType:         types.StringType,
						Computed:            true,
						MarkdownDescription: "Next scheduled fire times.",
					},
				},
			},
			"files":         replaceJSON("Optional JSON files configuration. Not returned by the API; adopted from config on import, and a change forces replacement."),
			"github":        replaceJSON("Optional JSON GitHub configuration. Not returned by the API; adopted from config on import, and a change forces replacement."),
			"memory_stores": replaceJSON("Optional JSON memory-stores configuration. Not returned by the API; adopted from config on import, and a change forces replacement."),
			"vaults":        replaceJSON("Optional JSON vaults configuration. Not returned by the API; adopted from config on import, and a change forces replacement."),
			"paused": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether scheduled triggers are suppressed. Toggling this pauses/unpauses in place (no replacement). The API may auto-pause on errors.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Deployment status (`active`, `paused`, ...).",
			},
			"paused_reason": schema.StringAttribute{
				CustomType:          jsontypes.NormalizedType{},
				Computed:            true,
				MarkdownDescription: "JSON object describing why the deployment is paused, or null.",
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *deploymentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *deploymentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deploymentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sched := client.Schedule{
		Type:       scheduleType(plan.Schedule),
		Expression: plan.Schedule.Expression.ValueString(),
		Timezone:   plan.Schedule.Timezone.ValueString(),
	}

	dep, err := r.client.CreateDeployment(ctx, client.DeploymentCreateRequest{
		Name:          plan.Name.ValueString(),
		Agent:         plan.AgentID.ValueString(),
		EnvironmentID: plan.EnvironmentID.ValueString(),
		InitialEvents: rawFromNormalized(plan.InitialEvents),
		Schedule:      sched,
		Files:         rawFromNormalized(plan.Files),
		GitHub:        rawFromNormalized(plan.GitHub),
		MemoryStores:  rawFromNormalized(plan.MemoryStores),
		Vaults:        rawFromNormalized(plan.Vaults),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating deployment", err.Error())
		return
	}

	// Honor an explicit paused=true by pausing right after creation.
	if !plan.Paused.IsNull() && !plan.Paused.IsUnknown() && plan.Paused.ValueBool() {
		paused, perr := r.client.PauseDeployment(ctx, dep.ID)
		if perr != nil {
			resp.Diagnostics.AddError("Error pausing deployment", perr.Error())
			return
		}
		dep = paused
	}

	r.applyComputed(ctx, &plan, dep, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *deploymentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deploymentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dep, err := r.client.GetDeployment(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading deployment", err.Error())
		return
	}

	state.Name = types.StringValue(dep.Name)
	r.applyComputed(ctx, &state, dep, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *deploymentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state deploymentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only `paused` is updatable in place; all else is RequiresReplace.
	dep, err := r.client.GetDeployment(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading deployment", err.Error())
		return
	}

	want := plan.Paused.ValueBool()
	if !plan.Paused.IsNull() && !plan.Paused.IsUnknown() && want != statusToPaused(dep.Status) {
		if want {
			dep, err = r.client.PauseDeployment(ctx, state.ID.ValueString())
		} else {
			dep, err = r.client.UnpauseDeployment(ctx, state.ID.ValueString())
		}
		if err != nil {
			resp.Diagnostics.AddError("Error changing deployment pause state", err.Error())
			return
		}
	}

	plan.Name = state.Name
	r.applyComputed(ctx, &plan, dep, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *deploymentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deploymentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.ArchiveDeployment(ctx, state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error archiving deployment", err.Error())
		return
	}
	resp.Diagnostics.AddWarning(
		"Deployment archived, not deleted",
		"Deployments cannot be hard-deleted. The deployment has been archived; its schedule is terminated and it cannot be modified.",
	)
}

func (r *deploymentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyComputed refreshes server-owned fields plus the schedule sub-object.
func (r *deploymentResource) applyComputed(ctx context.Context, m *deploymentResourceModel, d *client.Deployment, diags *diag.Diagnostics) {
	m.ID = types.StringValue(d.ID)
	m.Type = types.StringValue(d.Type)
	m.Status = types.StringValue(d.Status)
	m.Paused = types.BoolValue(statusToPaused(d.Status))
	m.CreatedAt = types.StringValue(d.CreatedAt)

	// Refresh agent_id/environment_id when the API returns them (GET does; the
	// create echo may not). Guarded so we never clobber the configured value
	// with an empty string.
	if aid := agentIDFromRaw(d.Agent); aid != "" {
		m.AgentID = types.StringValue(aid)
	}
	if d.EnvironmentID != "" {
		m.EnvironmentID = types.StringValue(d.EnvironmentID)
	}

	if d.PausedReason == nil {
		m.PausedReason = jsontypes.NewNormalizedNull()
	} else {
		b, err := json.Marshal(d.PausedReason)
		if err != nil {
			diags.AddError("Error encoding paused_reason", err.Error())
		} else {
			m.PausedReason = normalizedFromRaw(b)
		}
	}

	upcoming, d2 := stringsToList(ctx, d.Schedule.UpcomingRunsAt)
	diags.Append(d2...)
	if m.Schedule == nil {
		m.Schedule = &deploymentScheduleBlock{}
	}
	m.Schedule.Type = types.StringValue(d.Schedule.Type)
	m.Schedule.Expression = types.StringValue(d.Schedule.Expression)
	m.Schedule.Timezone = types.StringValue(d.Schedule.Timezone)
	m.Schedule.LastRunAt = stringFromPtr(d.Schedule.LastRunAt)
	m.Schedule.UpcomingRunsAt = upcoming
}

func scheduleType(b *deploymentScheduleBlock) string {
	if b == nil || b.Type.IsNull() || b.Type.IsUnknown() || b.Type.ValueString() == "" {
		return "cron"
	}
	return b.Type.ValueString()
}

func statusToPaused(status string) bool { return status == "paused" }

// agentIDFromRaw extracts the agent id from the deployment's `agent` field,
// which the API returns as an object ({"type":"agent","id":"...","version":N})
// but also accepts as a bare string.
func agentIDFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.ID != "" {
		return obj.ID
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}
