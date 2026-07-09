resource "claude_environment" "data_analysis" {
  name = "data-analysis"

  config = {
    type = "cloud"

    packages = {
      pip = ["pandas", "numpy", "scikit-learn"]
      npm = ["express"]
    }

    networking = {
      type                   = "limited"
      allowed_hosts          = ["api.example.com", "*.internal.example.com"]
      allow_mcp_servers      = true
      allow_package_managers = true
    }
  }
}

# A minimal environment with full outbound access.
resource "claude_environment" "sandbox" {
  name = "python-dev"

  config = {
    type = "cloud"
    networking = {
      type = "unrestricted"
    }
  }
}
