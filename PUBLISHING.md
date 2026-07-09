# Publishing to the Terraform Registry

The provider lives at
[`madewithlove/terraform-provider-claude-managed-agents`](https://github.com/madewithlove/terraform-provider-claude-managed-agents)
and publishes to `registry.terraform.io/madewithlove/claude-managed-agents`.

Releases are cut by GitHub Actions ([`.github/workflows/release.yml`](.github/workflows/release.yml)):
pushing a `v*` tag runs GoReleaser, which cross-compiles, zips, generates a
`SHA256SUMS` file, GPG-signs it, and attaches everything to a GitHub Release.
The registry ingests that release.

## One-time setup

### 1. Add the GPG signing secrets

The release signs the checksums with your GPG key. Add both as repository
secrets (Settings → Secrets and variables → Actions), or via the CLI:

```bash
# ASCII-armored private key (the whole block, including the header/footer lines)
gh secret set GPG_PRIVATE_KEY \
  --repo madewithlove/terraform-provider-claude-managed-agents \
  < private-key.asc

# The key's passphrase (set to an empty string if the key has none)
printf '%s' 'YOUR_PASSPHRASE' | gh secret set PASSPHRASE \
  --repo madewithlove/terraform-provider-claude-managed-agents
```

Export the matching **public** key for the registry (next step):

```bash
gpg --armor --export YOUR_KEY_ID > public-key.asc
```

### 2. Register the provider on the Terraform Registry

This step is a manual web action and can only be done by a `madewithlove`
GitHub org owner:

1. Sign in to <https://registry.terraform.io> with GitHub.
2. **Publish → Provider**, authorize the Terraform Registry GitHub app for the
   `madewithlove` org if prompted, and select
   `terraform-provider-claude-managed-agents`.
3. Paste the **public** GPG key (`public-key.asc`) when asked for a signing key.
4. Confirm. The registry will pick up existing and future releases.

## Cutting a release

Once the secrets are set:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Watch the run:

```bash
gh run watch --repo madewithlove/terraform-provider-claude-managed-agents
```

When it finishes, the GitHub Release holds the signed artifacts and the
registry publishes the version (usually within a few minutes) at
<https://registry.terraform.io/providers/madewithlove/claude-managed-agents>.

Follow semver for subsequent releases (`v0.1.1`, `v0.2.0`, …). Because the API
is a beta, staying on `0.x` signals that the schema may still change.

## Using the published provider

```hcl
terraform {
  required_providers {
    claude = {
      source  = "madewithlove/claude-managed-agents"
      version = "~> 0.1"
    }
  }
}
```

## Regenerating docs before a release

The `docs/` directory is generated from schema descriptions and the `examples/`
files. Regenerate after schema changes:

```bash
go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
tfplugindocs generate --provider-name claude
```
