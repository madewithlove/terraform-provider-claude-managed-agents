package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// These acceptance tests exercise the real Terraform binary against the
// provider talking to an in-process mock of the Claude API. They verify that
// imported resources plan cleanly: agent and environment produce an empty plan,
// and deployment produces an in-place adoption (never a destroy/recreate). They
// also confirm a managed create->apply->plan is a no-op and does not bump the
// agent version.
//
// Run with: TF_ACC=1 go test ./internal/provider -run TestAccImport
//
// No real API key or network access is required; base_url points at the mock.

// mockServer is a minimal, stateful mock of the endpoints the provider calls.
// It deliberately enriches JSON on the way back (adds default_config to tools,
// networking booleans to environment config) to reproduce the server-side
// enrichment that used to cause spurious import diffs.
func mockServer(t *testing.T) *httptest.Server {
	t.Helper()

	var (
		agentTools   json.RawMessage
		agentSystem  = "You are a helpful coding agent."
		agentVersion = 1
		envConfig    map[string]any
		depAgent     string
		depEnvID     string
	)

	readBody := func(r *http.Request) map[string]any {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux := http.NewServeMux()

	// ---- Agent ----
	enrichedTools := json.RawMessage(`[{"type":"agent_toolset_20260401","default_config":{"permission_policy":{"type":"always_allow"}}}]`)
	agentBody := func() map[string]any {
		return map[string]any{
			"id":          "agent_test",
			"type":        "agent",
			"name":        "Coding Assistant",
			"model":       map[string]any{"id": "claude-opus-4-8", "speed": "standard"},
			"system":      agentSystem,
			"description": nil,
			"tools":       agentTools,
			"skills":      []any{},
			"mcp_servers": []any{},
			"multiagent":  nil,
			"metadata":    map[string]any{},
			"version":     agentVersion,
			"created_at":  "2026-04-03T18:24:10.412Z",
			"updated_at":  "2026-04-03T18:24:10.412Z",
			"archived_at": nil,
		}
	}
	mux.HandleFunc("/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		body := readBody(r)
		if v, ok := body["system"].(string); ok {
			agentSystem = v
		}
		agentTools = enrichedTools
		write(w, agentBody())
	})
	mux.HandleFunc("/v1/agents/agent_test", func(w http.ResponseWriter, r *http.Request) {
		if agentTools == nil {
			agentTools = enrichedTools
		}
		if r.Method == http.MethodPost {
			// Update: bump version and apply a changed system prompt.
			body := readBody(r)
			if v, ok := body["system"].(string); ok && v != agentSystem {
				agentSystem = v
				agentVersion++
			}
		}
		write(w, agentBody())
	})
	mux.HandleFunc("/v1/agents/agent_test/archive", func(w http.ResponseWriter, r *http.Request) {
		b := agentBody()
		b["archived_at"] = "2026-04-04T00:00:00Z"
		write(w, b)
	})

	// ---- Environment ----
	envBody := func() map[string]any {
		return map[string]any{
			"id":          "env_test",
			"type":        "environment",
			"name":        "python-dev",
			"config":      envConfig,
			"created_at":  "2026-04-03T18:24:10.412Z",
			"updated_at":  "2026-04-03T18:24:10.412Z",
			"archived_at": nil,
		}
	}
	mux.HandleFunc("/v1/environments", func(w http.ResponseWriter, r *http.Request) {
		// Enrich: echo config but fill networking booleans the API defaults.
		envConfig = map[string]any{
			"type": "cloud",
			"networking": map[string]any{
				"type":                   "limited",
				"allowed_hosts":          []any{"api.example.com"},
				"allow_mcp_servers":      false,
				"allow_package_managers": false,
			},
		}
		write(w, envBody())
	})
	mux.HandleFunc("/v1/environments/env_test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if envConfig == nil {
			envConfig = map[string]any{
				"type": "cloud",
				"networking": map[string]any{
					"type":                   "limited",
					"allowed_hosts":          []any{"api.example.com"},
					"allow_mcp_servers":      false,
					"allow_package_managers": false,
				},
			}
		}
		write(w, envBody())
	})

	// ---- Deployment ----
	depBody := func() map[string]any {
		return map[string]any{
			"id":             "depl_test",
			"type":           "deployment",
			"name":           "Weekly scan",
			"status":         "active",
			"agent":          map[string]any{"type": "agent", "id": depAgent, "version": 1},
			"environment_id": depEnvID,
			"schedule": map[string]any{
				"type":             "cron",
				"expression":       "0 20 * * 5",
				"timezone":         "America/New_York",
				"last_run_at":      nil,
				"upcoming_runs_at": []any{"2026-05-09T00:00:00Z"},
			},
			"paused_reason": nil,
			"created_at":    "2026-04-03T18:24:10.412Z",
		}
	}
	mux.HandleFunc("/v1/deployments", func(w http.ResponseWriter, r *http.Request) {
		body := readBody(r)
		if v, ok := body["agent"].(string); ok {
			depAgent = v
		}
		if v, ok := body["environment_id"].(string); ok {
			depEnvID = v
		}
		write(w, depBody())
	})
	mux.HandleFunc("/v1/deployments/depl_test", func(w http.ResponseWriter, r *http.Request) {
		if depAgent == "" {
			depAgent = "agent_test"
		}
		if depEnvID == "" {
			depEnvID = "env_test"
		}
		write(w, depBody())
	})
	mux.HandleFunc("/v1/deployments/depl_test/archive", func(w http.ResponseWriter, r *http.Request) {
		write(w, depBody())
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testProviderFactories(baseURL string) map[string]func() (tfprotov6.ProviderServer, error) {
	_ = baseURL
	return map[string]func() (tfprotov6.ProviderServer, error){
		"claude": providerserver.NewProtocol6WithError(New("acctest")()),
	}
}

func providerConfig(baseURL string) string {
	return fmt.Sprintf(`
provider "claude" {
  api_key  = "test"
  base_url = %q
}
`, baseURL)
}

const agentCfgBody = `
resource "claude_agent" "test" {
  name   = "Coding Assistant"
  system = "You are a helpful coding agent."
  model  = { id = "claude-opus-4-8" }
  tools  = jsonencode([{ type = "agent_toolset_20260401" }])
}
`

const envCfgBody = `
resource "claude_environment" "test" {
  name = "python-dev"
  config = {
    type = "cloud"
    networking = {
      type          = "limited"
      allowed_hosts = ["api.example.com"]
    }
  }
}
`

const deploymentCfgBody = `
resource "claude_deployment" "test" {
  name           = "Weekly scan"
  agent_id       = "agent_test"
  environment_id = "env_test"
  initial_events = jsonencode([{ type = "user.message", content = [{ type = "text", text = "go" }] }])
  schedule = {
    expression = "0 20 * * 5"
    timezone   = "America/New_York"
  }
}
`

// TestAccAgentManagedNoop proves a managed create -> apply -> plan is a no-op
// (the API enriches tools on read, and subset semantics keep it clean) and that
// re-applying does not bump the agent version.
func TestAccAgentManagedNoop(t *testing.T) {
	srv := mockServer(t)
	cfg := providerConfig(srv.URL) + agentCfgBody
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr("claude_agent.test", "version", "1"),
			},
		},
	})
}

// TestAccAgentUpdateBumpsVersion proves ModifyPlan does not wrongly pin the
// version: a genuine change (system prompt) still updates in place and the
// version increments without an "inconsistent result" error.
func TestAccAgentUpdateBumpsVersion(t *testing.T) {
	srv := mockServer(t)
	base := providerConfig(srv.URL)
	cfgV1 := base + agentCfgBody
	cfgV2 := base + `
resource "claude_agent" "test" {
  name   = "Coding Assistant"
  system = "You are a helpful coding agent. Always write tests."
  model  = { id = "claude-opus-4-8" }
  tools  = jsonencode([{ type = "agent_toolset_20260401" }])
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{Config: cfgV1, Check: resource.TestCheckResourceAttr("claude_agent.test", "version", "1")},
			{
				Config: cfgV2,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("claude_agent.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("claude_agent.test", "version", "2"),
			},
		},
	})
}

// TestAccImportAgentPlansClean imports an agent that exists server-side and
// asserts the immediately-following plan is empty despite server enrichment.
func TestAccImportAgentPlansClean(t *testing.T) {
	srv := mockServer(t)
	cfg := providerConfig(srv.URL) + agentCfgBody
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       "claude_agent.test",
				ImportState:        true,
				ImportStateId:      "agent_test",
				ImportStatePersist: true,
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// multiMCPServer mocks an agent with two MCP servers whose tools and
// mcp_servers arrays are returned REORDERED (relative to config order) and
// ENRICHED with server-added keys — the exact shape that caused a spurious
// diff before order-insensitive equality.
func multiMCPServer(t *testing.T) *httptest.Server {
	t.Helper()
	toolsEnriched := json.RawMessage(`[
      {"type":"agent_toolset_20260401","default_config":{"enabled":true},"configs":[]},
      {"type":"mcp_toolset","mcp_server_name":"gcal-calendarmcp","default_config":{"enabled":true},"configs":[]},
      {"type":"mcp_toolset","mcp_server_name":"Team","default_config":{"enabled":true},"configs":[]}
    ]`)
	mcpEnriched := json.RawMessage(`[
      {"mcp_server_name":"gcal-calendarmcp","url":"https://gcal.example/mcp","configs":[]},
      {"mcp_server_name":"Team","url":"https://team.example/mcp","configs":[]}
    ]`)
	body := func() map[string]any {
		return map[string]any{
			"id":          "agent_multi",
			"type":        "agent",
			"name":        "Multi",
			"model":       map[string]any{"id": "claude-opus-4-8", "speed": "standard"},
			"system":      nil,
			"description": nil,
			"tools":       toolsEnriched,
			"mcp_servers": mcpEnriched,
			"skills":      []any{},
			"multiagent":  nil,
			"metadata":    map[string]any{},
			"version":     1,
			"created_at":  "2026-04-03T18:24:10.412Z",
			"updated_at":  "2026-04-03T18:24:10.412Z",
			"archived_at": nil,
		}
	}
	mux := http.NewServeMux()
	h := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(body())
	}
	mux.HandleFunc("/v1/agents", h)
	mux.HandleFunc("/v1/agents/agent_multi", h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestAccImportMultiMCPAgentPlansClean is the v0.2.1 regression: an agent with
// two MCP servers whose tools/mcp_servers come back reordered and enriched must
// import to an empty plan (no tools/mcp_servers/version diff).
func TestAccImportMultiMCPAgentPlansClean(t *testing.T) {
	srv := multiMCPServer(t)
	cfg := providerConfig(srv.URL) + `
resource "claude_agent" "multi" {
  name  = "Multi"
  model = { id = "claude-opus-4-8" }
  tools = jsonencode([
    { type = "agent_toolset_20260401" },
    { type = "mcp_toolset", mcp_server_name = "Team" },
    { type = "mcp_toolset", mcp_server_name = "gcal-calendarmcp" },
  ])
  mcp_servers = jsonencode([
    { mcp_server_name = "Team", url = "https://team.example/mcp" },
    { mcp_server_name = "gcal-calendarmcp", url = "https://gcal.example/mcp" },
  ])
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       "claude_agent.multi",
				ImportState:        true,
				ImportStateId:      "agent_multi",
				ImportStatePersist: true,
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccEnvironmentManagedNoop(t *testing.T) {
	srv := mockServer(t)
	cfg := providerConfig(srv.URL) + envCfgBody
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccImportEnvironmentPlansClean imports an environment and asserts the
// following plan is empty — no destroy/recreate despite config enrichment.
func TestAccImportEnvironmentPlansClean(t *testing.T) {
	srv := mockServer(t)
	cfg := providerConfig(srv.URL) + envCfgBody
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       "claude_environment.test",
				ImportState:        true,
				ImportStateId:      "env_test",
				ImportStatePersist: true,
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccImportDeploymentNoReplace imports a deployment and asserts the plan is
// an in-place adoption of the API-unreturnable fields (initial_events, ...),
// never a destroy/recreate.
func TestAccImportDeploymentNoReplace(t *testing.T) {
	srv := mockServer(t)
	cfg := providerConfig(srv.URL) + deploymentCfgBody
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config:             cfg,
				ResourceName:       "claude_deployment.test",
				ImportState:        true,
				ImportStateId:      "depl_test",
				ImportStatePersist: true,
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("claude_deployment.test", plancheck.ResourceActionUpdate),
					},
				},
			},
		},
	})
}
