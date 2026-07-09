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
	_ resource.Resource                = (*vaultResource)(nil)
	_ resource.ResourceWithConfigure   = (*vaultResource)(nil)
	_ resource.ResourceWithImportState = (*vaultResource)(nil)
)

type vaultResource struct {
	client *client.Client
}

// NewVaultResource is the resource factory.
func NewVaultResource() resource.Resource { return &vaultResource{} }

type vaultResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Type        types.String `tfsdk:"type"`
	DisplayName types.String `tfsdk:"display_name"`
	Metadata    types.Map    `tfsdk:"metadata"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
	ArchivedAt  types.String `tfsdk:"archived_at"`
}

func (r *vaultResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault"
}

func (r *vaultResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A vault: a workspace-scoped collection of credentials for one end user, referenced by ID when creating sessions. " +
			"`terraform destroy` hard-deletes the vault (falling back to archive if a session still references it).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"display_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable name for the vault (1-255 characters).",
			},
			"metadata": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Arbitrary key-value pairs, e.g. to map the vault to your own user records.",
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

func (r *vaultResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vaultResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vaultResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metadata, diags := mapToStringMap(ctx, plan.Metadata)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault, err := r.client.CreateVault(ctx, client.VaultCreateRequest{
		DisplayName: plan.DisplayName.ValueString(),
		Metadata:    metadata,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating vault", err.Error())
		return
	}

	applyVault(ctx, &plan, vault, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vaultResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vaultResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault, err := r.client.GetVault(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading vault", err.Error())
		return
	}

	applyVault(ctx, &state, vault, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vaultResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vaultResourceModel
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

	displayName := plan.DisplayName.ValueString()
	vault, err := r.client.UpdateVault(ctx, state.ID.ValueString(), client.VaultUpdateRequest{
		DisplayName: &displayName,
		Metadata:    metadata,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating vault", err.Error())
		return
	}

	applyVault(ctx, &plan, vault, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vaultResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vaultResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	err := r.client.DeleteVault(ctx, id)
	if err == nil || client.IsNotFound(err) {
		return
	}
	if client.IsConflict(err) {
		if _, aerr := r.client.ArchiveVault(ctx, id); aerr != nil {
			resp.Diagnostics.AddError("Error archiving vault", aerr.Error())
			return
		}
		resp.Diagnostics.AddWarning(
			"Vault archived instead of deleted",
			"The vault could not be hard-deleted because it is still referenced, so it was archived instead. Its secrets are purged; the record is retained for auditing.",
		)
		return
	}
	resp.Diagnostics.AddError("Error deleting vault", err.Error())
}

func (r *vaultResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func applyVault(ctx context.Context, m *vaultResourceModel, v *client.Vault, diags *diag.Diagnostics) {
	m.ID = types.StringValue(v.ID)
	m.Type = types.StringValue(v.Type)
	m.DisplayName = types.StringValue(v.DisplayName)
	m.CreatedAt = types.StringValue(v.CreatedAt)
	m.UpdatedAt = types.StringValue(v.UpdatedAt)
	m.ArchivedAt = stringFromPtr(v.ArchivedAt)
	if len(v.Metadata) == 0 {
		m.Metadata = types.MapNull(types.StringType)
		return
	}
	mv, d := stringMapToMap(ctx, v.Metadata)
	diags.Append(d...)
	m.Metadata = mv
}
