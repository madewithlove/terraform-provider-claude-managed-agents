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
	_ datasource.DataSource              = (*memoryStoreDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*memoryStoreDataSource)(nil)
)

type memoryStoreDataSource struct {
	client *client.Client
}

// NewMemoryStoreDataSource is the data-source factory.
func NewMemoryStoreDataSource() datasource.DataSource { return &memoryStoreDataSource{} }

func (d *memoryStoreDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_memory_store"
}

func (d *memoryStoreDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing memory store by ID.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Required: true, MarkdownDescription: "Memory store identifier."},
			"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "Object type."},
			"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Store name."},
			"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Store description."},
			"metadata":    schema.MapAttribute{ElementType: types.StringType, Computed: true, MarkdownDescription: "Metadata."},
			"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Last-update timestamp."},
			"archived_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Archive timestamp, or null."},
		},
	}
}

func (d *memoryStoreDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *memoryStoreDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data memoryStoreResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	store, err := d.client.GetMemoryStore(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading memory store", err.Error())
		return
	}

	applyMemoryStore(ctx, &data, store, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
