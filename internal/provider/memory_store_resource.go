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
)

var (
	_ resource.Resource                = (*memoryStoreResource)(nil)
	_ resource.ResourceWithConfigure   = (*memoryStoreResource)(nil)
	_ resource.ResourceWithImportState = (*memoryStoreResource)(nil)
)

type memoryStoreResource struct {
	client *client.Client
}

// NewMemoryStoreResource is the resource factory.
func NewMemoryStoreResource() resource.Resource { return &memoryStoreResource{} }

type memoryStoreResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Type        types.String `tfsdk:"type"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Metadata    types.Map    `tfsdk:"metadata"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
	ArchivedAt  types.String `tfsdk:"archived_at"`
}

func (r *memoryStoreResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_memory_store"
}

func (r *memoryStoreResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A memory store: a workspace-scoped, named container for agent memories, mounted into sessions to persist state across runs. " +
			"`terraform destroy` permanently deletes the store and all of its memories (falling back to archive if it is still referenced).",
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
				MarkdownDescription: "Human-readable name (1-255 chars). The `/mnt/memory/` mount slug is derived from this.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "What the store contains (up to 1024 chars). Included in the agent's system prompt when attached.",
			},
			"metadata": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Arbitrary key-value tags (max 16; not visible to the agent).",
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at":  schema.StringAttribute{Computed: true},
			"archived_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Archive timestamp (RFC 3339), or null if active."},
		},
	}
}

func (r *memoryStoreResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *memoryStoreResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memoryStoreResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metadata, diags := mapToStringMap(ctx, plan.Metadata)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	store, err := r.client.CreateMemoryStore(ctx, client.MemoryStoreCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: stringPtr(plan.Description),
		Metadata:    metadata,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating memory store", err.Error())
		return
	}

	applyMemoryStore(ctx, &plan, store, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memoryStoreResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memoryStoreResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	store, err := r.client.GetMemoryStore(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading memory store", err.Error())
		return
	}

	applyMemoryStore(ctx, &state, store, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *memoryStoreResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state memoryStoreResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metadata, diags := mergedMetadata(ctx, state.Metadata, plan.Metadata)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	// Send description unconditionally so clearing it in config clears it
	// server-side (the API clears with an empty string).
	description := plan.Description.ValueString()

	store, err := r.client.UpdateMemoryStore(ctx, state.ID.ValueString(), client.MemoryStoreUpdateRequest{
		Name:        &name,
		Description: &description,
		Metadata:    metadata,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating memory store", err.Error())
		return
	}

	applyMemoryStore(ctx, &plan, store, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memoryStoreResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memoryStoreResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	err := r.client.DeleteMemoryStore(ctx, id)
	if err == nil || client.IsNotFound(err) {
		return
	}
	if client.IsConflict(err) {
		if _, aerr := r.client.ArchiveMemoryStore(ctx, id); aerr != nil {
			resp.Diagnostics.AddError("Error archiving memory store", aerr.Error())
			return
		}
		resp.Diagnostics.AddWarning(
			"Memory store archived instead of deleted",
			"The store could not be deleted because it is still referenced, so it was archived (read-only) instead.",
		)
		return
	}
	resp.Diagnostics.AddError("Error deleting memory store", err.Error())
}

func (r *memoryStoreResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func applyMemoryStore(ctx context.Context, m *memoryStoreResourceModel, s *client.MemoryStore, diags *diag.Diagnostics) {
	m.ID = types.StringValue(s.ID)
	m.Type = types.StringValue(s.Type)
	m.Name = types.StringValue(s.Name)
	m.CreatedAt = types.StringValue(s.CreatedAt)
	m.UpdatedAt = types.StringValue(s.UpdatedAt)
	m.ArchivedAt = stringFromPtr(s.ArchivedAt)

	// The API returns "" for an unset description; normalize to null so a
	// config that omits it does not perpetually diff.
	if s.Description == nil || *s.Description == "" {
		m.Description = types.StringNull()
	} else {
		m.Description = types.StringValue(*s.Description)
	}

	if len(s.Metadata) == 0 {
		m.Metadata = types.MapNull(types.StringType)
		return
	}
	mv, d := stringMapToMap(ctx, s.Metadata)
	diags.Append(d...)
	m.Metadata = mv
}
