package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ datasource.DataSource              = (*agentDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*agentDataSource)(nil)
)

type agentDataSource struct {
	client *client.Client
}

// NewAgentDataSource is the data-source factory.
func NewAgentDataSource() datasource.DataSource { return &agentDataSource{} }

type agentDataSourceModel struct {
	ID          types.String         `tfsdk:"id"`
	Type        types.String         `tfsdk:"type"`
	Name        types.String         `tfsdk:"name"`
	Model       *agentModelBlock     `tfsdk:"model"`
	System      types.String         `tfsdk:"system"`
	Description types.String         `tfsdk:"description"`
	Tools       jsontypes.Normalized `tfsdk:"tools"`
	Skills      jsontypes.Normalized `tfsdk:"skills"`
	MCPServers  jsontypes.Normalized `tfsdk:"mcp_servers"`
	Multiagent  jsontypes.Normalized `tfsdk:"multiagent"`
	Metadata    types.Map            `tfsdk:"metadata"`
	Version     types.Int64          `tfsdk:"version"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
	ArchivedAt  types.String         `tfsdk:"archived_at"`
}

func (d *agentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_agent"
}

func (d *agentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Claude agent by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Agent identifier.",
			},
			"type": schema.StringAttribute{Computed: true, MarkdownDescription: "Object type."},
			"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Agent name."},
			"model": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Model configuration.",
				Attributes: map[string]schema.Attribute{
					"id":    schema.StringAttribute{Computed: true},
					"speed": schema.StringAttribute{Computed: true},
				},
			},
			"system":      schema.StringAttribute{Computed: true, MarkdownDescription: "System prompt."},
			"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Description."},
			"tools":       schema.StringAttribute{CustomType: jsontypes.NormalizedType{}, Computed: true, MarkdownDescription: "Tools (JSON)."},
			"mcp_servers": schema.StringAttribute{CustomType: jsontypes.NormalizedType{}, Computed: true, MarkdownDescription: "MCP servers (JSON)."},
			"skills":      schema.StringAttribute{CustomType: jsontypes.NormalizedType{}, Computed: true, MarkdownDescription: "Skills (JSON)."},
			"multiagent":  schema.StringAttribute{CustomType: jsontypes.NormalizedType{}, Computed: true, MarkdownDescription: "Multi-agent coordinator (JSON)."},
			"metadata":    schema.MapAttribute{ElementType: types.StringType, Computed: true, MarkdownDescription: "Metadata."},
			"version":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Current version."},
			"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Last-update timestamp."},
			"archived_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Archive timestamp, or null."},
		},
	}
}

func (d *agentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *agentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data agentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	agent, err := d.client.GetAgent(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading agent", err.Error())
		return
	}

	data.Type = types.StringValue(agent.Type)
	data.Name = types.StringValue(agent.Name)
	data.Model = &agentModelBlock{ID: types.StringValue(agent.Model.ID), Speed: types.StringValue(agent.Model.Speed)}
	data.System = stringFromPtr(agent.System)
	data.Description = stringFromPtr(agent.Description)
	data.Tools = normalizedFromRaw(agent.Tools)
	data.MCPServers = normalizedFromRaw(agent.MCPServers)
	data.Skills = normalizedFromRaw(agent.Skills)
	data.Multiagent = normalizedFromRaw(agent.Multiagent)
	if len(agent.Metadata) == 0 {
		data.Metadata = types.MapNull(types.StringType)
	} else {
		mv, diags := stringMapToMap(ctx, agent.Metadata)
		resp.Diagnostics.Append(diags...)
		data.Metadata = mv
	}
	data.Version = types.Int64Value(agent.Version)
	data.CreatedAt = types.StringValue(agent.CreatedAt)
	data.UpdatedAt = types.StringValue(agent.UpdatedAt)
	data.ArchivedAt = stringFromPtr(agent.ArchivedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
