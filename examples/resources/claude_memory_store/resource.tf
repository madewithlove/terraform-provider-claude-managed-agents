resource "claude_memory_store" "prefs" {
  name        = "User Preferences"
  description = "Per-user preferences and project context."
  metadata = {
    external_user_id = "usr_abc123"
  }
}

# Seed the store with read-only reference material before any agent runs.
resource "claude_memory" "formatting_standards" {
  memory_store_id = claude_memory_store.prefs.id
  path            = "/formatting_standards.md"
  content         = <<-EOT
    All reports use GAAP formatting.
    Dates are ISO-8601.
  EOT
}

output "memory_store_id" {
  value = claude_memory_store.prefs.id
}
