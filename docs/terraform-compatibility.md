# Terraform HCL Compatibility

Pulumi HCL allows you to write infrastructure as code using Terraform-compatible HCL syntax while leveraging the Pulumi engine for state management, secrets, and deployments.

**Goal**: Maximum compatibility with Terraform HCL syntax with no gratuitous differences.

This document focuses on **language and semantic differences**—cases where valid HCL code may behave differently than in Terraform. Platform-level differences (state management, CLI commands, etc.) are expected when adopting any new IaC tool and are summarized briefly at the end.

## Supported HCL Syntax

The following Terraform HCL constructs are fully supported with identical behavior:

| Feature | Support | Notes |
|---------|---------|-------|
| `resource` blocks | Full | Including all meta-arguments |
| `data` source blocks | Full | Invoked via Pulumi functions |
| `variable` blocks | Full | Type constraints, defaults, nullable, sensitive |
| `locals` blocks | Full | Expression interpolation supported |
| `output` blocks | Full | value, description, sensitive, depends_on |
| `provider` blocks | Full | Including alias support |
| `terraform.required_providers` | Full | Provider version constraints |
| `count` meta-argument | Full | Index expansion with `count.index` |
| `for_each` meta-argument | Full | Key expansion with `each.key`, `each.value` |
| `depends_on` | Full | Explicit dependencies |
| Built-in functions | 80+ | Most Terraform functions supported |
| Splat expressions | Full | `resource.name[*].attr` |
| String interpolation | Full | `"${var.name}-suffix"` |
| Complex types | Full | Lists, maps, sets, objects, tuples |
| Resource references | Full | `aws_instance.web.id`, `data.aws_ami.ubuntu.id` |

## Language and Semantic Differences

These are cases where valid HCL syntax is interpreted differently by Pulumi HCL. Understanding these differences is important when migrating existing configurations.

### Lifecycle Block Behavior

The `lifecycle` block is supported, but not all options have an effect:

| Lifecycle Option | Status | Behavior |
|------------------|--------|----------|
| `prevent_destroy` | Works | Maps to Pulumi's `protect` resource option |
| `ignore_changes` | Works | Maps to Pulumi's `ignoreChanges` resource option |
| `create_before_destroy` | No effect | Pulumi uses its own replacement ordering logic |
| `replace_triggered_by` | No effect | Parsed but ignored |
| `precondition` / `postcondition` | No effect | Parsed but validations do not run |

For `create_before_destroy`: Pulumi's engine determines replacement strategy based on resource type and provider behavior. You cannot override this ordering via HCL.

For `replace_triggered_by`: If you depend on this to force resource replacement when dependencies change, you'll need to restructure your configuration or trigger replacements manually.

### Provider Source Names

Provider sources use the `pulumi/` namespace rather than `hashicorp/`:

```hcl
terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"  # Not "hashicorp/aws"
      version = ">= 6.0"
    }
  }
}
```

Terraform-style resource type names (e.g., `aws_instance`) are automatically mapped to Pulumi tokens for bridged providers. For native Pulumi providers without a Terraform bridge, use Pulumi-style type names directly:

```hcl
# Bridged provider - Terraform-style works
resource "aws_instance" "web" { }

# Native Pulumi provider - use Pulumi-style
resource "kubernetes:apps/v1:Deployment" "app" { }
```

### Secrets and Sensitive Values

Pulumi has a richer secrets model than Terraform. Values marked `sensitive` are automatically wrapped as Pulumi secrets, which provides encryption at rest in state:

```hcl
variable "database_password" {
  type      = string
  sensitive = true  # Becomes a Pulumi secret (encrypted in state)
}
```

This is generally an improvement, but be aware that secret values propagate differently—any output derived from a secret is also marked secret.

### Ignored Terraform Blocks

The following blocks are parsed for compatibility but have no effect, since Pulumi handles these concerns differently:

```hcl
terraform {
  # Ignored - Pulumi manages state via Pulumi Cloud or self-managed backends
  backend "s3" {
    bucket = "my-terraform-state"
  }

  # Ignored - use Pulumi Cloud instead
  cloud {
    organization = "my-org"
  }

  # Ignored - Pulumi has its own versioning
  required_version = ">= 1.0"
}
```

No warning is emitted for these blocks, so existing configurations with backend definitions will work without modification.

## Not Yet Implemented

These features are planned but not yet available. They represent temporary gaps rather than fundamental incompatibilities.

### Modules

Module blocks are parsed but produce an error at runtime:

```hcl
# NOT YET SUPPORTED - will error
module "vpc" {
  source = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}
```

**Workaround**: Inline the module's resources directly into your configuration.

### Provisioners

Provisioner blocks (`local-exec`, `remote-exec`, `file`) are parsed but silently ignored at runtime:

```hcl
resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  # SILENTLY IGNORED
  provisioner "remote-exec" {
    inline = ["sudo apt-get update"]
  }
}
```

**Workaround**: Use the [Pulumi Command provider](https://www.pulumi.com/registry/packages/command/):

```hcl
resource "command:remote:Command" "setup" {
  connection {
    host = aws_instance.web.public_ip
  }
  create = "sudo apt-get update"
}
```

### Variable Configuration

Variables currently only use default values defined in HCL. Pulumi stack configuration (`pulumi config set`) is not yet integrated:

```hcl
variable "region" {
  type    = string
  default = "us-west-2"  # Only this default is used
}
```

**Workaround**: Set values via environment variables (`TF_VAR_region=us-east-1`) or ensure all variables have appropriate defaults.

### Variable Validation

Validation blocks are parsed but not enforced:

```hcl
variable "instance_type" {
  type    = string
  default = "t3.micro"

  # PARSED BUT NOT ENFORCED
  validation {
    condition     = can(regex("^t3\\.", var.instance_type))
    error_message = "Must be a t3 instance type."
  }
}
```

### `moved` and `import` Blocks

These Terraform 1.x features for state manipulation are not supported:

```hcl
# NOT SUPPORTED
moved {
  from = aws_instance.old_name
  to   = aws_instance.new_name
}

# NOT SUPPORTED
import {
  to = aws_instance.web
  id = "i-1234567890abcdef0"
}
```

**Workaround**: Use `pulumi state rename` and `pulumi import` commands.

### Missing Function

| Function | Status |
|----------|--------|
| `rsadecrypt()` | Not implemented |

## Platform Differences

These are expected differences when using any new IaC platform. They affect how you operate the tool, not how your HCL code behaves.

| Concern | Terraform | Pulumi HCL |
|---------|-----------|------------|
| **State storage** | Local files, S3, Terraform Cloud | Pulumi Cloud or self-managed backends |
| **State commands** | `terraform state list/show/rm` | `pulumi state` |
| **Import resources** | `terraform import` or `import` blocks | `pulumi import` |
| **Workspaces** | `terraform workspace` | Pulumi stacks |
| **Preview changes** | `terraform plan` | `pulumi preview` |
| **Apply changes** | `terraform apply` | `pulumi up` |
| **Destroy** | `terraform destroy` | `pulumi destroy` |

Existing Terraform state files are not compatible with Pulumi. When migrating, either import existing resources with `pulumi import` or recreate them.

## Quick Reference

| Feature | Status | Notes |
|---------|:------:|-------|
| Resources | Yes | Full support |
| Data sources | Yes | Full support |
| Variables | Partial | Defaults only; stack config pending |
| Locals | Yes | Full support |
| Outputs | Yes | Full support |
| Providers | Yes | Use `pulumi/` source prefix |
| `count` / `for_each` | Yes | Full support |
| `depends_on` | Yes | Full support |
| `lifecycle` | Partial | `prevent_destroy` and `ignore_changes` work; others ignored |
| Modules | No | Not yet implemented |
| Provisioners | No | Silently ignored; use Command provider |
| `moved` / `import` blocks | No | Use CLI commands |
| Functions | Mostly | 80+ supported; `rsadecrypt` pending |

## Getting Help

If you encounter compatibility issues not covered here:

1. Check the [Pulumi HCL GitHub repository](https://github.com/pulumi/pulumi-language-hcl) for known issues
2. File an issue with a minimal reproduction case
3. Join the [Pulumi Community Slack](https://slack.pulumi.com/) for community support
