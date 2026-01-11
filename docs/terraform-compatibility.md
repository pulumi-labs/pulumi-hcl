# Terraform HCL Compatibility

Pulumi HCL allows you to write infrastructure as code using Terraform-compatible HCL syntax while leveraging the Pulumi engine for state management, secrets, and deployments.

**Goal**: Maximum compatibility with Terraform HCL syntax with no gratuitous differences.

This document describes what's supported, what's not yet implemented, and what platform-level differences to expect when using Pulumi instead of Terraform.

## Fully Supported

The following Terraform HCL constructs work with identical behavior:

### Core Constructs

| Feature | Notes |
|---------|-------|
| `resource` blocks | Full support including nested blocks |
| `data` source blocks | Invoked via Pulumi provider functions |
| `variable` blocks | Type constraints, defaults, nullable, sensitive |
| `locals` blocks | Full expression interpolation |
| `output` blocks | value, description, sensitive, depends_on |
| `provider` blocks | Including alias support |
| `terraform.required_providers` | Provider version constraints |

### Meta-Arguments

| Feature | Notes |
|---------|-------|
| `count` | Index expansion with `count.index` |
| `for_each` | Key expansion with `each.key`, `each.value` |
| `depends_on` | Explicit dependency declarations |
| `provider` | Provider selection per resource |

### Lifecycle Options

| Option | Notes |
|--------|-------|
| `prevent_destroy` | Maps to Pulumi's `protect` resource option |
| `ignore_changes` | Maps to Pulumi's `ignoreChanges` resource option |

### Expressions and Functions

| Feature | Notes |
|---------|-------|
| String interpolation | `"${var.name}-suffix"` |
| Splat expressions | `resource.name[*].attr` |
| Resource references | `aws_instance.web.id`, `data.aws_ami.ubuntu.id` |
| Complex types | Lists, maps, sets, objects, tuples |
| Built-in functions | 80+ Terraform functions supported |

### Sensitive Values

Values marked `sensitive` are automatically wrapped as Pulumi secrets, providing encryption at rest:

```hcl
variable "database_password" {
  type      = string
  sensitive = true  # Encrypted in Pulumi state
}
```

## Not Yet Implemented

These features are parsed for compatibility but not yet functional. They represent temporary implementation gaps.

### Lifecycle Options

| Option | Status |
|--------|--------|
| `create_before_destroy` | Parsed but not yet wired to Pulumi's [`deleteBeforeReplace`](https://www.pulumi.com/docs/iac/concepts/resources/options/deletebeforereplace/) |
| `replace_triggered_by` | Not supported (no direct Pulumi equivalent) |
| `precondition` / `postcondition` | Parsed but validations do not run |

**Note on `replace_triggered_by`**: Terraform's `replace_triggered_by` cascades replacement when *other* resources change. Pulumi's [`replaceOnChanges`](https://www.pulumi.com/docs/iac/concepts/resources/options/replaceonchanges/) triggers replacement when properties on *this* resource change—different semantics that don't directly map.

### Modules

Module blocks produce an error at runtime:

```hcl
# NOT YET SUPPORTED - will error
module "vpc" {
  source = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}
```

**Workaround**: Inline the module's resources directly into your configuration.

### Provisioners

Provisioner blocks (`local-exec`, `remote-exec`, `file`) are not supported and will produce a warning:

```hcl
resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  # NOT SUPPORTED
  provisioner "remote-exec" {
    inline = ["sudo apt-get update"]
  }
}
```

**Workaround**: Use the [Pulumi Command provider](https://www.pulumi.com/registry/packages/command/).

### Variable Configuration

Variables currently only use default values from HCL. Pulumi stack configuration (`pulumi config set`) is not yet integrated:

```hcl
variable "region" {
  type    = string
  default = "us-west-2"  # Only this default is used
}
```

**Workaround**: Set values via environment variables (`TF_VAR_region=us-east-1`) or ensure all variables have defaults.

### Variable Validation

Validation blocks are not yet enforced (will produce a warning):

```hcl
variable "instance_type" {
  type    = string
  default = "t3.micro"

  # NOT YET ENFORCED
  validation {
    condition     = can(regex("^t3\\.", var.instance_type))
    error_message = "Must be a t3 instance type."
  }
}
```

### State Migration Blocks

`moved` and `import` blocks are not supported:

```hcl
# NOT SUPPORTED
moved {
  from = aws_instance.old_name
  to   = aws_instance.new_name
}
```

**Workaround**: Use `pulumi state rename` and `pulumi import` CLI commands.

### Missing Functions

| Function | Status |
|----------|--------|
| `rsadecrypt()` | Not yet implemented |

## Platform Differences

These are expected differences when using Pulumi instead of Terraform. They don't affect HCL syntax or behavior—just how you operate the tool.

### Provider Configuration

Provider sources use Pulumi's namespace:

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

The following blocks are parsed but have no effect (Pulumi handles these concerns differently):

```hcl
terraform {
  backend "s3" { }        # Ignored - use Pulumi state backends
  cloud { }               # Ignored - use Pulumi Cloud
  required_version = ""   # Ignored - Pulumi has its own versioning
}
```

### CLI and State

| Concern | Terraform | Pulumi HCL |
|---------|-----------|------------|
| **State storage** | Local files, S3, Terraform Cloud | Pulumi Cloud or self-managed backends |
| **State commands** | `terraform state` | `pulumi state` |
| **Import resources** | `terraform import` | `pulumi import` |
| **Workspaces** | `terraform workspace` | Pulumi stacks |
| **Preview changes** | `terraform plan` | `pulumi preview` |
| **Apply changes** | `terraform apply` | `pulumi up` |
| **Destroy** | `terraform destroy` | `pulumi destroy` |

Existing Terraform state files are not compatible. When migrating, import resources with `pulumi import` or recreate them.

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
| `prevent_destroy` | Yes | Maps to `protect` |
| `ignore_changes` | Yes | Maps to `ignoreChanges` |
| `create_before_destroy` | Pending | Parsed but not yet functional |
| `replace_triggered_by` | No | No direct Pulumi equivalent |
| `precondition` / `postcondition` | Pending | Parsed but not enforced |
| Modules | No | Not yet implemented |
| Provisioners | No | Not supported; use Command provider |
| `moved` / `import` blocks | No | Use CLI commands |
| Functions | Mostly | 80+ supported; `rsadecrypt` pending |

## Getting Help

If you encounter compatibility issues not covered here:

1. Check the [Pulumi HCL GitHub repository](https://github.com/pulumi/pulumi-language-hcl) for known issues
2. File an issue with a minimal reproduction case
3. Join the [Pulumi Community Slack](https://slack.pulumi.com/) for community support
