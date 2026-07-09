# Terraform Provider for Claude Managed Agents

A Terraform provider for [Claude Managed Agents](https://platform.claude.com/docs/en/managed-agents/overview) — Anthropic's managed harness for running Claude as an autonomous agent. It manages the durable, declarative objects of the API:

| Resource | API object | Notes |
| --- | --- | --- |
| `claude_agent` | Agent | Versioned config: model, system prompt, tools, MCP servers, skills |
| `claude_environment` | Environment | Sandbox config (cloud / self-hosted); immutable |
| `claude_deployment` | Scheduled deployment | Runs an agent on a cron schedule |
| `claude_vault` | Vault | Per-end-user collection of credentials referenced at session creation |
| `claude_vault_credential` | Vault credential | MCP OAuth / static bearer / environment-variable auth; **write-only secrets** |
| `claude_memory_store` | Memory store | Named container for agent memories, mounted into sessions |
| `claude_memory` | Memory | A text document in a store; for seeding read-only reference content |

Data sources: `claude_agent`, `claude_environment`, `claude_vault`, `claude_memory_store`.

> Sessions and events are runtime/ephemeral and are intentionally **not** modeled as Terraform resources.

> **Beta.** The Managed Agents API is in beta and requires the `managed-agents-2026-04-01` beta header (the provider sends it automatically). Shapes may change between releases.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/dl/) >= 1.23 (to build)
- A [Claude API key](https://platform.claude.com/settings/keys)

## Provider configuration

```hcl
terraform {
  required_providers {
    claude = {
      source = "madewithlove/claude-managed-agents"
    }
  }
}

provider "claude" {
  # api_key defaults to $ANTHROPIC_API_KEY.
  # api_key           = "sk-ant-..."
  # base_url          = "https://api.anthropic.com"   # or $ANTHROPIC_BASE_URL
  # anthropic_version = "2023-06-01"
}
```

| Argument | Env fallback | Default |
| --- | --- | --- |
| `api_key` | `ANTHROPIC_API_KEY` | — (required) |
| `base_url` | `ANTHROPIC_BASE_URL` | `https://api.anthropic.com` |
| `anthropic_version` | — | `2023-06-01` |

## Usage

```hcl
resource "claude_agent" "coding" {
  name   = "Coding Assistant"
  system = "You are a helpful coding agent. Always write tests."
  model  = { id = "claude-opus-4-8" }         # add speed = "fast" for fast mode
  tools  = jsonencode([{ type = "agent_toolset_20260401" }])
  metadata = { team = "platform" }
}

resource "claude_environment" "sandbox" {
  name = "python-dev"
  config = {
    type       = "cloud"
    packages   = { pip = ["pandas", "numpy"] }
    networking = { type = "unrestricted" }
  }
}

resource "claude_deployment" "weekly_scan" {
  name           = "Weekly compliance scan"
  agent_id       = claude_agent.coding.id
  environment_id = claude_environment.sandbox.id

  initial_events = jsonencode([
    { type = "user.message", content = [{ type = "text", text = "Run the weekly compliance scan." }] }
  ])

  schedule = {
    expression = "0 20 * * 5"   # Fridays 20:00
    timezone   = "America/New_York"
  }

  paused = false   # toggle in place; no replacement
}
```

### Vaults and credentials

```hcl
resource "claude_vault" "alice" {
  display_name = "Alice"
  metadata     = { external_user_id = "usr_abc123" }
}

resource "claude_vault_credential" "linear" {
  vault_id     = claude_vault.alice.id
  display_name = "Linear API key"

  auth = {
    type           = "static_bearer"
    mcp_server_url = "https://mcp.linear.app/mcp"
    token          = var.linear_api_key # write-only: never stored in state
  }

  secret_version = "1" # bump to re-send the token after rotating it
}
```

The `auth` block is a flattened union discriminated by `type`:

- **`mcp_oauth`** — `mcp_server_url`, `access_token` (write-only), `expires_at`, optional `refresh { token_endpoint, client_id, scope, refresh_token (write-only), token_endpoint_auth { type, client_secret (write-only) } }`
- **`static_bearer`** — `mcp_server_url`, `token` (write-only)
- **`environment_variable`** — `secret_name`, `secret_value` (write-only), `networking { type, allowed_hosts }`, `injection_location { header, body }`

### Memory stores

```hcl
resource "claude_memory_store" "prefs" {
  name        = "User Preferences"
  description = "Per-user preferences and project context."
}

# Seed read-only reference content before any agent runs.
resource "claude_memory" "standards" {
  memory_store_id = claude_memory_store.prefs.id
  path            = "/formatting_standards.md"
  content         = "All reports use GAAP formatting. Dates are ISO-8601."
}
```

More in [`examples/`](./examples).

## Design decisions & behavior

These follow directly from the shape of the API. Read them before relying on the provider in production.

### Agents are versioned; destroy archives (never deletes)

- Updates use **optimistic concurrency**: the provider sends the state's `version`. If the agent changed outside Terraform, the API returns 409 and the apply fails with a message telling you to `terraform apply -refresh-only` first.
- The API has no delete endpoint. `terraform destroy` **archives** the agent (read-only; existing sessions keep running) and emits a warning.
- `metadata` is merged key-by-key by the API. The provider makes it behave declaratively: keys you remove from config are explicitly nulled out on update so they are deleted server-side.

### Complex union fields are JSON

`tools`, `mcp_servers`, `skills`, and `multiagent` (on agents) and `initial_events`, `files`, `github`, `memory_stores`, `vaults` (on deployments) are large, evolving beta union types. Rather than model every variant, they are accepted as JSON strings — use `jsonencode(...)`. Whitespace and key order are normalized, so formatting differences don't cause spurious diffs.

Because the API enriches some of these on read (e.g. adding a tool's `default_config`), the provider **does not refresh them from the API**. Out-of-band changes to those fields are therefore not detected as drift. The values you write are the source of truth.

### Environments are immutable

The API has no environment update endpoint, so **any** change to `name` or `config` forces replacement. `config` is not refreshed on read (to avoid churn from server-populated defaults). On `terraform destroy` the provider deletes the environment; if a session still references it (delete returns 409), it falls back to **archiving** it and warns.

### Deployments are immutable except `paused`

Every deployment field forces replacement **except** `paused`, which pauses/unpauses in place via the dedicated endpoints. The API may auto-pause a deployment on errors; a subsequent `terraform plan` will reconcile `paused` back to your configured value. `terraform destroy` archives the deployment (terminal).

### Vault credential secrets are write-only

Secret values (`access_token`, `token`, `secret_value`, `refresh_token`, `client_secret`) use Terraform [write-only arguments](https://developer.hashicorp.com/terraform/language/resources/ephemeral/write-only) — they are sent to the API on create/update but **never stored in state or plan**, and the API never returns them. Because Terraform can't see a change to a write-only value, rotating a secret in config does nothing on its own; **bump `secret_version`** to force the provider to re-send the current config secrets. (Requires Terraform ≥ 1.11.)

Structural keys — `auth.type`, `auth.mcp_server_url`, `auth.secret_name`, `auth.refresh.token_endpoint`, `auth.refresh.client_id`, and `vault_id` — are immutable and force replacement. `display_name`, `metadata`, `scope`, `expires_at`, `networking`, `injection_location`, and the secret values (with a `secret_version` bump) update in place. Credentials import as `<vault_id>/<credential_id>`.

Vaults hard-delete on destroy, falling back to archive if a session still references them (secrets are purged either way).

### Memory stores are for config; memories are for seeding

`claude_memory_store` manages the store's declarative config (`name`, `description`, `metadata`). `terraform destroy` permanently deletes the store and all its memories (archive fallback if still referenced; archive is one-way).

`claude_memory` is best for **seeding stores with read-only reference content**. Agents write to `read_write` stores at runtime, so a memory Terraform manages in such a store will show the agent's out-of-band edits as drift on the next plan. Content updates send a `content_sha256` precondition, so if an agent changed the content since the last refresh, the apply fails safely rather than clobbering it — run `terraform apply -refresh-only` to adopt the current content first. Memories import as `<memory_store_id>/<memory_id>`.

## Development

```bash
make build      # compile ./terraform-provider-claude-managed-agents
make test       # unit tests
make vet        # go vet
make validate   # install + validate example config against the schema (no API calls)
```

### Local testing with dev overrides

```bash
make install    # go install into $GOBIN
```

Create `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "madewithlove/claude-managed-agents" = "/Users/you/go/bin"
  }
  direct {}
}
```

Then run Terraform against a real config with `ANTHROPIC_API_KEY` set. With dev overrides you skip `terraform init`.

## License

MPL-2.0 (see `LICENSE`).
