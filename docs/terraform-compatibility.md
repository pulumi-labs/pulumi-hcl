# Terraform HCL Compatibility

Pulumi HCL supports the majority of [Terraform's HCL syntax](https://developer.hashicorp.com/terraform/language). Resources, data sources, variables, locals, outputs, modules, expressions, and functions broadly work as documented by HashiCorp.

This document covers what's different and what's not supported.

## The One Required Change

Provider sources must use the `pulumi/` namespace instead of `hashicorp/`:

```hcl
terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"  # not "hashicorp/aws"
      version = ">= 6.0"
    }
  }
}
```

## Behavioral Differences

### Resource Replacement Order

Terraform deletes a resource before creating its replacement. Pulumi does the opposite: it creates the new resource first, then deletes the old one. This minimizes downtime but can cause issues with unique constraints like DNS names or ports.

To get Terraform's delete-first behavior:

```hcl
lifecycle {
  create_before_destroy = false
}
```

Setting `create_before_destroy = false` explicitly triggers Pulumi's `deleteBeforeReplace` option.

### Sensitive Values

Variables and outputs marked `sensitive = true` become Pulumi secrets, which are encrypted at rest in Pulumi's state. This provides stronger protection than Terraform's state handling.

### Variable Configuration

Variables support Pulumi's configuration system alongside Terraform's:

1. Environment variables (`TF_VAR_name`) — highest priority
2. Pulumi stack config (`pulumi config set projectName:varName value`)
3. Default values in `variable` blocks — lowest priority

## Feature Mappings

These Terraform features map to Pulumi equivalents:

| Terraform               | Pulumi                 | Notes                                 |
|-------------------------|------------------------|---------------------------------------|
| `prevent_destroy`       | `protect`              | Same behavior                         |
| `ignore_changes`        | `ignoreChanges`        | Same behavior                         |
| `create_before_destroy` | `deleteBeforeReplace`  | Inverted logic (see above)            |
| `moved` blocks          | `aliases`              | Renames without recreation            |
| `import` blocks         | Import resource option | Imports existing resources            |
| `timeouts`              | `customTimeouts`       | Same duration format                  |
| Modules                 | Component resources    | Full support including remote sources |
| Provisioners            | Command provider       | `local-exec`, `remote-exec`, `file`   |

### Modules

All module sources work:

- Local paths: `./modules/vpc`
- Git: `git::https://github.com/org/repo.git//path?ref=v1.0.0`
- GitHub shorthand: `github.com/org/repo`
- Terraform Registry: `terraform-aws-modules/vpc/aws`
- HTTP archives: `https://example.com/module.zip`

Remote modules are cached in `~/.pulumi/modules/`.

### Provisioners

Provisioner blocks map to the [Command provider](https://www.pulumi.com/registry/packages/command/):

| Terraform     | Pulumi                        |
|---------------|-------------------------------|
| `local-exec`  | `command:local:Command`       |
| `remote-exec` | `command:remote:Command`      |
| `file`        | `command:remote:CopyToRemote` |

All provisioner features work: `self` references, `when = "destroy"`, `on_failure = "continue"`, and connection blocks. WinRM connections are not supported—SSH only.

## Ignored Blocks

These blocks are parsed but have no effect:

```hcl
terraform {
  backend "s3" { }        # Use pulumi login instead
  cloud { }               # Use Pulumi Cloud instead
  required_version = ""   # Pulumi has its own versioning
}
```

## Unsupported Features

**`replace_triggered_by`** — Terraform's lifecycle option cascades replacement when *other* resources change. Pulumi's [`replaceOnChanges`](https://www.pulumi.com/docs/iac/concepts/resources/options/replaceonchanges/) triggers replacement when properties on *this* resource change. These have different semantics and don't map directly.

**`dynamic` blocks** — Dynamic block generation (`dynamic "tag" { for_each = ... content { ... } }`) is not implemented.

**`List<Object>` empty vs null** — HCL's block syntax cannot distinguish between an empty list and a null `List<Object>`. Programs that rely on this distinction (e.g. passing `null` vs `[]` for a block-typed attribute) will not behave correctly.

**WinRM connections** — `connection` blocks only support `type = "ssh"`. WinRM is not supported.

## CLI Reference

| Terraform           | Pulumi           |
|---------------------|------------------|
| `terraform plan`    | `pulumi preview` |
| `terraform apply`   | `pulumi up`      |
| `terraform destroy` | `pulumi destroy` |
| `terraform state`   | `pulumi state`   |
| `terraform import`  | `pulumi import`  |
| Workspaces          | Stacks           |

Terraform state files are not compatible. Import existing resources with `pulumi import`.

## Built-in Functions

Pulumi HCL supports nearly all of Terraform's built-in functions with identical behavior. The sections below document the exceptions.

### Functions in Terraform but not supported here

| Function          | Category        | Notes                                                                         |
|-------------------|-----------------|-------------------------------------------------------------------------------|
| `templatestring`  | String          | Renders an inline template string with a given context object.                |
| `plantimestamp`   | Date and Time   | Returns the timestamp at the start of a plan, which has no Pulumi equivalent. |
| `ephemeralasnull` | Type Conversion | Replaces ephemeral values with `null`; Pulumi has no ephemeral value concept. |
| `issensitive`     | Type Conversion | Returns whether a value is marked sensitive.                                  |

The `provider::terraform::*` provider functions and `terraform.applying` are Terraform-internal and have no equivalent here.

### Functions supported here but not in Terraform

| Function             | Category        | Notes                                                             |
|----------------------|-----------------|-------------------------------------------------------------------|
| `entries`            | Collection      | Converts a map or object to a list of `{key, value}` objects.     |
| `pulumiResourceName` | Pulumi-specific | Returns the Pulumi resource name for a resource reference.        |
| `pulumiResourceType` | Pulumi-specific | Returns the Pulumi resource type for a resource reference.        |
| `fileAsset`          | Asset/Archive   | Creates a Pulumi `FileAsset` from a local file path.              |
| `stringAsset`        | Asset/Archive   | Creates a Pulumi `StringAsset` from a string value.               |
| `remoteAsset`        | Asset/Archive   | Creates a Pulumi `RemoteAsset` from a URL.                        |
| `fileArchive`        | Asset/Archive   | Creates a Pulumi `FileArchive` from a local path.                 |
| `assetArchive`       | Asset/Archive   | Creates a Pulumi `AssetArchive` from a map of assets or archives. |

## Getting Help

- [GitHub repository](https://github.com/pulumi/pulumi-language-hcl) for issues and source
- [Pulumi Community Slack](https://slack.pulumi.com/) for questions
