package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ resource.Resource                = (*memoryResource)(nil)
	_ resource.ResourceWithConfigure   = (*memoryResource)(nil)
	_ resource.ResourceWithImportState = (*memoryResource)(nil)
)

type memoryResource struct {
	client *client.Client
}

// NewMemoryResource is the resource factory.
func NewMemoryResource() resource.Resource { return &memoryResource{} }

type memoryResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Type             types.String `tfsdk:"type"`
	MemoryStoreID    types.String `tfsdk:"memory_store_id"`
	Path             types.String `tfsdk:"path"`
	Content          types.String `tfsdk:"content"`
	ContentSHA256    types.String `tfsdk:"content_sha256"`
	ContentSizeBytes types.Int64  `tfsdk:"content_size_bytes"`
	MemoryVersionID  types.String `tfsdk:"memory_version_id"`
	CreatedAt        types.String `tfsdk:"created_at"`
	UpdatedAt        types.String `tfsdk:"updated_at"`
}

func (r *memoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_memory"
}

func (r *memoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A single memory (text document at a path) inside a memory store. " +
			"Intended for **seeding** stores with reference material and managing read-only content. " +
			"Note: agents write to `read_write` stores at runtime, so managing memories in a store the agent also writes to will surface out-of-band changes as drift on the next plan.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"memory_store_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the memory store this memory belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Hierarchical path, e.g. `/preferences/formatting.md`. Must start with `/`. Changing it renames the memory in place.",
			},
			"content": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UTF-8 text content (max 100 kB). Pass an empty string for an empty memory.",
			},
			"content_sha256": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 of the content bytes. Used as an optimistic-concurrency precondition on update.",
			},
			"content_size_bytes": schema.Int64Attribute{Computed: true, MarkdownDescription: "Content size in bytes."},
			"memory_version_id":  schema.StringAttribute{Computed: true, MarkdownDescription: "ID of the current memory version."},
			"created_at":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"updated_at":         schema.StringAttribute{Computed: true},
		},
	}
}

func (r *memoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *memoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mem, err := r.client.CreateMemory(ctx, plan.MemoryStoreID.ValueString(), client.MemoryCreateRequest{
		Path:    plan.Path.ValueString(),
		Content: plan.Content.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating memory", err.Error())
		return
	}

	applyMemory(&plan, mem)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mem, err := r.client.GetMemory(ctx, state.MemoryStoreID.ValueString(), state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading memory", err.Error())
		return
	}

	applyMemory(&state, mem)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *memoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state memoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var updateReq client.MemoryUpdateRequest
	if !plan.Path.Equal(state.Path) {
		p := plan.Path.ValueString()
		updateReq.Path = &p
	}
	if !plan.Content.Equal(state.Content) {
		c := plan.Content.ValueString()
		updateReq.Content = &c
		// Guard against clobbering a concurrent (e.g. agent) write.
		if sha := state.ContentSHA256.ValueString(); sha != "" {
			updateReq.Precondition = &client.MemoryPrecondition{Type: "content_sha256", ContentSHA256: sha}
		}
	}

	mem, err := r.client.UpdateMemory(ctx, state.MemoryStoreID.ValueString(), state.ID.ValueString(), updateReq)
	if err != nil {
		if client.IsConflict(err) {
			resp.Diagnostics.AddError(
				"Memory content conflict",
				"The memory was modified outside Terraform (content hash precondition failed), likely by an agent session. "+
					"Run `terraform apply -refresh-only` to adopt the current content, then reapply.\n\n"+err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError("Error updating memory", err.Error())
		return
	}

	applyMemory(&plan, mem)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *memoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteMemory(ctx, state.MemoryStoreID.ValueString(), state.ID.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting memory", err.Error())
	}
}

// ImportState expects "<memory_store_id>/<memory_id>".
func (r *memoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	storeID, memID, ok := splitTwo(req.ID, "/")
	if !ok {
		resp.Diagnostics.AddError("Invalid import ID", "Expected \"<memory_store_id>/<memory_id>\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("memory_store_id"), storeID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memID)...)
}

func applyMemory(m *memoryResourceModel, mem *client.Memory) {
	m.ID = types.StringValue(mem.ID)
	m.Type = types.StringValue(mem.Type)
	m.MemoryStoreID = types.StringValue(mem.MemoryStoreID)
	m.Path = types.StringValue(mem.Path)
	m.Content = stringFromPtr(mem.Content)
	m.ContentSHA256 = types.StringValue(mem.ContentSHA256)
	m.ContentSizeBytes = types.Int64Value(mem.ContentSizeBytes)
	m.MemoryVersionID = types.StringValue(mem.MemoryVersionID)
	m.CreatedAt = types.StringValue(mem.CreatedAt)
	m.UpdatedAt = types.StringValue(mem.UpdatedAt)
}
