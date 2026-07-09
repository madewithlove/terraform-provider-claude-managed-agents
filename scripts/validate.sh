#!/usr/bin/env bash
# Validates the example configuration against the provider schema using
# terraform CLI dev_overrides. This exercises GetProviderSchema and config
# validation without making any API calls (no ANTHROPIC_API_KEY required).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOBIN="$(go env GOBIN)"
[ -z "$GOBIN" ] && GOBIN="$(go env GOPATH)/bin"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# Combined config referencing every resource and data source.
cat >"$WORKDIR/main.tf" <<'EOF'
terraform {
  required_providers {
    claude = {
      source = "madewithlove/claude-managed-agents"
    }
  }
}

provider "claude" {
  api_key = "placeholder"
}

resource "claude_agent" "a" {
  name   = "Coding Assistant"
  system = "You are a helpful coding agent."
  model  = { id = "claude-opus-4-8" }
  tools  = jsonencode([{ type = "agent_toolset_20260401" }])
  metadata = { team = "platform" }
}

resource "claude_environment" "e" {
  name = "python-dev"
  config = {
    type = "cloud"
    packages   = { pip = ["pandas"] }
    networking = { type = "limited", allowed_hosts = ["api.example.com"], allow_mcp_servers = true }
  }
}

resource "claude_deployment" "d" {
  name           = "Weekly scan"
  agent_id       = claude_agent.a.id
  environment_id = claude_environment.e.id
  initial_events = jsonencode([{ type = "user.message", content = [{ type = "text", text = "go" }] }])
  schedule = {
    expression = "0 20 * * 5"
    timezone   = "America/New_York"
  }
}

resource "claude_vault" "v" {
  display_name = "Alice"
  metadata     = { external_user_id = "usr_abc123" }
}

resource "claude_vault_credential" "bearer" {
  vault_id     = claude_vault.v.id
  display_name = "Linear"
  auth = {
    type           = "static_bearer"
    mcp_server_url = "https://mcp.linear.app/mcp"
    token          = "lin_api_placeholder"
  }
  secret_version = "1"
}

resource "claude_vault_credential" "envvar" {
  vault_id = claude_vault.v.id
  auth = {
    type         = "environment_variable"
    secret_name  = "NOTION_API_KEY"
    secret_value = "ntn_placeholder"
    networking   = { type = "limited", allowed_hosts = ["api.notion.com"] }
    injection_location = { header = true }
  }
}

resource "claude_memory_store" "ms" {
  name        = "User Preferences"
  description = "Per-user preferences."
  metadata    = { external_user_id = "usr_abc123" }
}

resource "claude_memory" "seed" {
  memory_store_id = claude_memory_store.ms.id
  path            = "/formatting_standards.md"
  content         = "All reports use GAAP formatting."
}

data "claude_agent" "look" {
  id = "agent_123"
}

data "claude_environment" "look" {
  id = "env_123"
}

data "claude_vault" "look" {
  id = "vlt_123"
}

data "claude_memory_store" "look" {
  id = "memstore_123"
}
EOF

cat >"$WORKDIR/dev.tfrc" <<EOF
provider_installation {
  dev_overrides {
    "madewithlove/claude-managed-agents" = "$GOBIN"
  }
  direct {}
}
EOF

echo "Validating example configuration against the provider schema..."
cd "$WORKDIR"
TF_CLI_CONFIG_FILE="$WORKDIR/dev.tfrc" terraform validate
