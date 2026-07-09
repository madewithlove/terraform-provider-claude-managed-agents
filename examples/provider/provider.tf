terraform {
  required_providers {
    claude = {
      source = "madewithlove/claude-managed-agents"
    }
  }
}

provider "claude" {
  # api_key defaults to the ANTHROPIC_API_KEY environment variable.
  # api_key = "sk-ant-..."
}
