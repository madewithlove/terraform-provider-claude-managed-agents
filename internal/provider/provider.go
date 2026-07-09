package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/madewithlove/terraform-provider-claude-managed-agents/internal/client"
)

// Ensure the implementation satisfies the provider.Provider interface.
var _ provider.Provider = (*claudeProvider)(nil)

// claudeProvider is the Managed Agents Terraform provider.
type claudeProvider struct {
	version string
}

// providerModel maps the provider configuration block.
type providerModel struct {
	APIKey           types.String `tfsdk:"api_key"`
	BaseURL          types.String `tfsdk:"base_url"`
	AnthropicVersion types.String `tfsdk:"anthropic_version"`
}

// New returns a provider factory for the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &claudeProvider{version: version}
	}
}

func (p *claudeProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	// Resource/data-source prefix. Registry source is
	// madewithlove/claude-managed-agents; the type name stays "claude" so
	// resources read claude_agent, claude_environment, claude_deployment.
	resp.TypeName = "claude"
	resp.Version = p.version
}

func (p *claudeProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Claude Managed Agents resources: agents, environments, scheduled deployments, vaults and credentials, and memory stores.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Claude API key. Falls back to the `ANTHROPIC_API_KEY` environment variable.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "API base URL. Falls back to `ANTHROPIC_BASE_URL`, then `https://api.anthropic.com`.",
			},
			"anthropic_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Value for the `anthropic-version` header. Defaults to `2023-06-01`.",
			},
		},
	}
}

func (p *claudeProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unknown values mean a dependency isn't resolved yet at plan time.
	if cfg.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			pathRoot("api_key"),
			"Unknown API key",
			"The api_key value is unknown at configuration time. Set it statically or via ANTHROPIC_API_KEY.",
		)
		return
	}

	apiKey := cfg.APIKey.ValueString()
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			pathRoot("api_key"),
			"Missing API key",
			"Set the api_key provider argument or the ANTHROPIC_API_KEY environment variable.",
		)
		return
	}

	baseURL := cfg.BaseURL.ValueString()
	if baseURL == "" {
		baseURL = os.Getenv("ANTHROPIC_BASE_URL")
	}

	opts := []client.Option{
		client.WithBaseURL(baseURL),
		client.WithUserAgent("terraform-provider-claude-managed-agents/" + p.version),
	}
	if !cfg.AnthropicVersion.IsNull() && cfg.AnthropicVersion.ValueString() != "" {
		opts = append(opts, client.WithAnthropicVersion(cfg.AnthropicVersion.ValueString()))
	}

	c := client.New(apiKey, opts...)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *claudeProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAgentResource,
		NewEnvironmentResource,
		NewDeploymentResource,
		NewVaultResource,
		NewVaultCredentialResource,
		NewMemoryStoreResource,
		NewMemoryResource,
	}
}

func (p *claudeProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAgentDataSource,
		NewEnvironmentDataSource,
		NewVaultDataSource,
		NewMemoryStoreDataSource,
	}
}
