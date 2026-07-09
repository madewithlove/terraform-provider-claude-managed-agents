resource "claude_deployment" "weekly_scan" {
  name           = "Weekly compliance scan"
  agent_id       = claude_agent.coding_assistant.id
  environment_id = claude_environment.sandbox.id

  initial_events = jsonencode([
    {
      type    = "user.message"
      content = [{ type = "text", text = "Run the weekly compliance scan." }]
    }
  ])

  schedule = {
    type       = "cron"
    expression = "0 20 * * 5"
    timezone   = "America/New_York"
  }

  # Toggle scheduled triggers in place (no replacement).
  paused = false
}

output "next_runs" {
  value = claude_deployment.weekly_scan.schedule.upcoming_runs_at
}
