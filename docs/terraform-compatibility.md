# Terraform HCL Compatibility

Pulumi HCL lets you write infrastructure using Terraform-compatible HCL syntax while running on the Pulumi engine. This document covers what works, what's different, and what's not yet implemented.

## Supported Features

The following HCL constructs work with identical behavior to Terraform:

### Core Blocks

| Block | Notes |
|-------|-------|
| `resource` | Full support including nested blocks |
| `data` | Data sources invoked via Pulumi provider functions |
| `variable` | Type constraints, defaults, nullable, sensitive |
| `locals` | Full expression interpolation |
| `output` | value, description, sensitive, depends_on |
| `provider` | Including alias support |
| `terraform.required_providers` | Provider version constraints |

### Meta-Arguments

| Argument | Notes |
|----------|-------|
| `count` | Index expansion with `count.index` |
| `for_each` | Key expansion with `each.key`, `each.value` |
| `depends_on` | Explicit dependency declarations |
| `provider` | Provider selection per resource |

### Lifecycle Options

| Option | Pulumi Equivalent |
|--------|-------------------|
| `prevent_destroy` | [`protect`](https://www.pulumi.com/docs/iac/concepts/resources/options/protect/) |
| `ignore_changes` | [`ignoreChanges`](https://www.pulumi.com/docs/iac/concepts/resources/options/ignorechanges/) |

### Expressions

| Feature | Notes |
|---------|-------|
| String interpolation | `"${var.name}-suffix"` |
| Splat expressions | `resource.name[*].attr` |
| Resource references | `aws_instance.web.id`, `data.aws_ami.ubuntu.id` |
| Complex types | Lists, maps, sets, objects, tuples |
| Built-in functions | 80+ Terraform functions |

### Sensitive Values

Values marked `sensitive` become Pulumi secrets, providing encryption at rest:

```hcl
variable "database_password" {
  type      = string
  sensitive = true  # Encrypted in Pulumi state
}
```

## Platform Differences

These are expected differences when using Pulumi instead of Terraform. They affect tooling, not HCL behavior.

### Provider Sources

Use the `pulumi/` namespace for providers:

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

Terraform-style resource type names (e.g., `aws_instance`) are automatically mapped to Pulumi tokens for bridged providers.

### Ignored Blocks

These blocks are parsed but have no effect:

```hcl
terraform {
  backend "s3" { }        # Use Pulumi state backends instead
  cloud { }               # Use Pulumi Cloud instead
  required_version = ""   # Pulumi has its own versioning
}
```

### CLI Commands

| Operation | Terraform | Pulumi |
|-----------|-----------|--------|
| Preview changes | `terraform plan` | `pulumi preview` |
| Apply changes | `terraform apply` | `pulumi up` |
| Destroy | `terraform destroy` | `pulumi destroy` |
| State operations | `terraform state` | `pulumi state` |
| Import resources | `terraform import` | `pulumi import` |
| Environments | `terraform workspace` | Pulumi stacks |

Existing Terraform state files are not compatible. Import resources with `pulumi import` or recreate them.

## Unsupported Features

These features cannot be supported due to fundamental differences between Pulumi and Terraform.

### `replace_triggered_by`

Terraform's `replace_triggered_by` cascades replacement when *other* resources change. Pulumi's [`replaceOnChanges`](https://www.pulumi.com/docs/iac/concepts/resources/options/replaceonchanges/) triggers replacement when properties on *this* resource change—different semantics that don't map directly.

### `moved` and `import` Blocks

Use `pulumi state rename` and `pulumi import` CLI commands instead.

## Not Yet Implemented

These features are parsed for compatibility but not yet functional. They represent temporary gaps.

### Lifecycle Options

| Option | Status |
|--------|--------|
| `create_before_destroy` | Will map to [`deleteBeforeReplace`](https://www.pulumi.com/docs/iac/concepts/resources/options/deletebeforereplace/) |
| `precondition` / `postcondition` | Validations do not run yet |

### Modules

Module blocks produce an error at runtime:

```hcl
module "vpc" {
  source     = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}
# Error: module support not yet implemented
```

**Workaround**: Inline the module's resources directly.

### Provisioners

Provisioner blocks (`local-exec`, `remote-exec`, `file`) are parsed but not executed. They will map to the [Pulumi Command provider](https://www.pulumi.com/registry/packages/command/):

```hcl
provisioner "remote-exec" {
  inline = ["sudo apt-get update"]
}
# Parsed but not yet executed
```

### Variable Configuration

Variables use default values only. Pulumi stack configuration (`pulumi config set`) is not yet integrated:

```hcl
variable "region" {
  type    = string
  default = "us-west-2"  # Only this default is used
}
```

**Workaround**: Use environment variables (`TF_VAR_region=us-east-1`) or ensure all variables have defaults.

### Variable Validation

Validation blocks are parsed but not enforced:

```hcl
variable "instance_type" {
  type    = string
  default = "t3.micro"

  validation {
    condition     = can(regex("^t3\\.", var.instance_type))
    error_message = "Must be a t3 instance type."
  }
}
# Validation is not enforced
```

### Missing Functions

| Function | Status |
|----------|--------|
| `rsadecrypt()` | Not yet implemented |

## Quick Reference

| Feature | Status | Notes |
|---------|:------:|-------|
| Resources | Yes | |
| Data sources | Yes | |
| Variables | Partial | Defaults only; stack config pending |
| Locals | Yes | |
| Outputs | Yes | |
| Providers | Yes | Use `pulumi/` source prefix |
| `count` / `for_each` | Yes | |
| `depends_on` | Yes | |
| `prevent_destroy` | Yes | Maps to `protect` |
| `ignore_changes` | Yes | Maps to `ignoreChanges` |
| `create_before_destroy` | Pending | Will map to `deleteBeforeReplace` |
| `replace_triggered_by` | No | No direct Pulumi equivalent |
| `precondition` / `postcondition` | Pending | |
| Modules | Pending | Not yet implemented |
| Provisioners | Pending | Will map to Command provider |
| `moved` / `import` blocks | No | Use CLI commands |
| Functions | Mostly | 80+ supported; `rsadecrypt` pending |

## Getting Help

If you encounter compatibility issues:

1. Check the [GitHub repository](https://github.com/pulumi/pulumi-language-hcl) for known issues
2. File an issue with a minimal reproduction
3. Join the [Pulumi Community Slack](https://slack.pulumi.com/)
