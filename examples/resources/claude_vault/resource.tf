resource "claude_vault" "alice" {
  display_name = "Alice"
  metadata = {
    external_user_id = "usr_abc123"
  }
}

# MCP static bearer credential (e.g. Linear API key).
resource "claude_vault_credential" "linear" {
  vault_id     = claude_vault.alice.id
  display_name = "Linear API key"

  auth = {
    type           = "static_bearer"
    mcp_server_url = "https://mcp.linear.app/mcp"
    token          = var.linear_api_key # write-only, never stored in state
  }

  # Bump to re-send the token after rotating it in your secret manager.
  secret_version = "1"
}

# Environment-variable credential, header-only, limited to one host.
resource "claude_vault_credential" "notion" {
  vault_id     = claude_vault.alice.id
  display_name = "Notion API key for sandbox"

  auth = {
    type         = "environment_variable"
    secret_name  = "NOTION_API_KEY"
    secret_value = var.notion_api_key # write-only

    networking = {
      type          = "limited"
      allowed_hosts = ["api.notion.com"]
    }

    injection_location = {
      header = true
    }
  }

  secret_version = "1"
}

# MCP OAuth credential with Anthropic-managed refresh.
resource "claude_vault_credential" "slack" {
  vault_id     = claude_vault.alice.id
  display_name = "Alice's Slack"

  auth = {
    type           = "mcp_oauth"
    mcp_server_url = "https://mcp.slack.com/mcp"
    access_token   = var.slack_access_token # write-only
    expires_at     = "2099-12-31T23:59:59Z"

    refresh = {
      token_endpoint = "https://slack.com/api/oauth.v2.access"
      client_id      = "1234567890.0987654321"
      scope          = "channels:read chat:write"
      refresh_token  = var.slack_refresh_token # write-only

      token_endpoint_auth = {
        type          = "client_secret_post"
        client_secret = var.slack_client_secret # write-only
      }
    }
  }

  secret_version = "1"
}

variable "linear_api_key" {
  type      = string
  sensitive = true
}
variable "notion_api_key" {
  type      = string
  sensitive = true
}
variable "slack_access_token" {
  type      = string
  sensitive = true
}
variable "slack_refresh_token" {
  type      = string
  sensitive = true
}
variable "slack_client_secret" {
  type      = string
  sensitive = true
}
