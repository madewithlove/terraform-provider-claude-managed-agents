package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ datasource.DataSource              = (*vaultDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*vaultDataSource)(nil)
)

type vaultDataSource struct {
	client *client.Client
}

// NewVaultDataSource is the data-source factory.
func NewVaultDataSource() datasource.DataSource { return &vaultDataSource{} }

func (d *vaultDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault"
}

func (d *vaultDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing vault by ID.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true, MarkdownDescription: "Vault identifier."},
			"type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Object type."},
			"display_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Vault display name."},
			"metadata":     schema.MapAttribute{ElementType: types.StringType, Computed: true, MarkdownDescription: "Metadata."},
			"created_at":   schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at":   schema.StringAttribute{Computed: true, MarkdownDescription: "Last-update timestamp."},
			"archived_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Archive timestamp, or null."},
		},
	}
}

func (d *vaultDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData))
		return
	}
	d.client = c
}

func (d *vaultDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data vaultResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault, err := d.client.GetVault(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading vault", err.Error())
		return
	}

	applyVault(ctx, &data, vault, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
