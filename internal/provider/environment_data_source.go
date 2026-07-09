package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

var (
	_ datasource.DataSource              = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*environmentDataSource)(nil)
)

type environmentDataSource struct {
	client *client.Client
}

// NewEnvironmentDataSource is the data-source factory.
func NewEnvironmentDataSource() datasource.DataSource { return &environmentDataSource{} }

func (d *environmentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (d *environmentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	stringList := func(desc string) schema.ListAttribute {
		return schema.ListAttribute{ElementType: types.StringType, Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing Claude Managed Agents environment by ID.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Required: true, MarkdownDescription: "Environment identifier."},
			"type": schema.StringAttribute{Computed: true, MarkdownDescription: "Object type."},
			"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Environment name."},
			"config": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Sandbox configuration.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{Computed: true},
					"packages": schema.SingleNestedAttribute{
						Computed: true,
						Attributes: map[string]schema.Attribute{
							"apt":   stringList("System packages."),
							"cargo": stringList("Rust crates."),
							"gem":   stringList("Ruby gems."),
							"go":    stringList("Go modules."),
							"npm":   stringList("Node.js packages."),
							"pip":   stringList("Python packages."),
						},
					},
					"networking": schema.SingleNestedAttribute{
						Computed: true,
						Attributes: map[string]schema.Attribute{
							"type":                   schema.StringAttribute{Computed: true},
							"allowed_hosts":          stringList("Allowed hosts."),
							"allow_mcp_servers":      schema.BoolAttribute{Computed: true},
							"allow_package_managers": schema.BoolAttribute{Computed: true},
						},
					},
				},
			},
			"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
			"updated_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Last-update timestamp."},
			"archived_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Archive timestamp, or null."},
		},
	}
}

func (d *environmentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *environmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data environmentResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	env, err := d.client.GetEnvironment(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading environment", err.Error())
		return
	}

	data.Name = types.StringValue(env.Name)
	cfg, diags := envConfigFromAPI(ctx, env.Config)
	resp.Diagnostics.Append(diags...)
	data.Config = cfg
	applyEnvComputed(&data, env)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// envConfigFromAPI maps an API environment config into the nested block model.
func envConfigFromAPI(ctx context.Context, c client.EnvironmentConfig) (*envConfigBlock, diag.Diagnostics) {
	var diags diag.Diagnostics
	block := &envConfigBlock{Type: types.StringValue(c.Type)}

	if c.Packages != nil {
		p := &envPackagesBlock{}
		var d diag.Diagnostics
		p.Apt, d = stringsToList(ctx, c.Packages.Apt)
		diags.Append(d...)
		p.Cargo, d = stringsToList(ctx, c.Packages.Cargo)
		diags.Append(d...)
		p.Gem, d = stringsToList(ctx, c.Packages.Gem)
		diags.Append(d...)
		p.Go, d = stringsToList(ctx, c.Packages.Go)
		diags.Append(d...)
		p.Npm, d = stringsToList(ctx, c.Packages.Npm)
		diags.Append(d...)
		p.Pip, d = stringsToList(ctx, c.Packages.Pip)
		diags.Append(d...)
		block.Packages = p
	}

	if c.Networking != nil {
		n := &envNetworkingBlock{Type: types.StringValue(c.Networking.Type)}
		hosts, d := stringsToList(ctx, c.Networking.AllowedHosts)
		diags.Append(d...)
		n.AllowedHosts = hosts
		n.AllowMCPServers = types.BoolPointerValue(c.Networking.AllowMCPServers)
		n.AllowPackageManagers = types.BoolPointerValue(c.Networking.AllowPackageManagers)
		block.Networking = n
	}

	return block, diags
}
