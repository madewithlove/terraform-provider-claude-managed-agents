data "claude_agent" "existing" {
  id = "agent_01HqR2k7vXbZ9mNpL3wYcT8f"
}

data "claude_environment" "existing" {
  id = "env_01abc"
}

output "agent_model" {
  value = data.claude_agent.existing.model.id
}
