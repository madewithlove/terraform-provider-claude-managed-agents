resource "claude_agent" "coding_assistant" {
  name   = "Coding Assistant"
  system = "You are a helpful coding agent. Always write tests."

  model = {
    id    = "claude-opus-4-8"
    speed = "standard"
  }

  # Complex, evolving union fields are supplied as JSON.
  tools = jsonencode([
    { type = "agent_toolset_20260401" }
  ])

  metadata = {
    team = "platform"
    env  = "production"
  }
}

output "agent_id" {
  value = claude_agent.coding_assistant.id
}

output "agent_version" {
  value = claude_agent.coding_assistant.version
}
