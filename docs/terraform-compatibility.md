# Terraform HCL Compatibility

Pulumi HCL allows you to write infrastructure as code using Terraform-compatible HCL syntax while leveraging the Pulumi engine for state management, secrets, and deployments. This document describes the compatibility between Pulumi HCL and Terraform HCL, including known differences and current limitations.

## Overview

Pulumi HCL parses standard HCL files and translates them to Pulumi resource registrations at runtime. While most Terraform HCL syntax is supported, there are important behavioral differences due to the underlying Pulumi engine.

**Goal**: Maximum compatibility with Terraform HCL syntax with no gratuitous differences.

**Reality**: Some differences are unavoidable due to fundamental architectural differences between Pulumi and Terraform.

## Supported Features

The following Terraform HCL constructs are fully supported:

| Feature | Support | Notes |
|---------|---------|-------|
| `resource` blocks | Full | Including all meta-arguments |
| `data` source blocks | Full | Invoked via Pulumi functions |
| `variable` blocks | Full | Type constraints, defaults, nullable, sensitive, validation |
| `locals` blocks | Full | Expression interpolation supported |
| `output` blocks | Full | value, description, sensitive, depends_on, preconditions |
| `provider` blocks | Full | Including alias support |
| `terraform.required_providers` | Full | Provider version constraints |
| `count` meta-argument | Full | Index expansion with `count.index` |
| `for_each` meta-argument | Full | Key expansion with `each.key`, `each.value` |
| `depends_on` | Full | Explicit dependencies |
| `lifecycle` block | Partial | See [Lifecycle Differences](#lifecycle-block-differences) |
| Built-in functions | 80+ | Most Terraform functions supported |
| Splat expressions | Full | `resource.name[*].attr` |
| String interpolation | Full | `"${var.name}-suffix"` |
| Complex types | Full | Lists, maps, sets, objects, tuples |

## Fundamental Differences

These differences arise from architectural distinctions between Pulumi and Terraform and represent permanent behavioral variations.

### State Management

| Aspect | Terraform | Pulumi HCL |
|--------|-----------|------------|
| State storage | Local files, S3, Terraform Cloud, etc. | Pulumi Cloud or self-managed backends |
| State format | Terraform state JSON | Pulumi checkpoint format |
| State migration | `terraform state` commands | `pulumi state` commands |

**Impact**: Existing Terraform state files cannot be directly imported. Resources must be imported using `pulumi import` or recreated.

The following Terraform blocks are parsed but have no effect:

```hcl
terraform {
  # Ignored - Pulumi manages state
  backend "s3" {
    bucket = "my-terraform-state"
    key    = "state.tfstate"
  }

  # Ignored - Pulumi has its own cloud service
  cloud {
    organization = "my-org"
  }

  # Ignored - Pulumi has its own versioning
  required_version = ">= 1.0"
}
```

### Execution Model

| Aspect | Terraform | Pulumi HCL |
|--------|-----------|------------|
| Planning | Separate plan phase (`terraform plan`) | Integrated preview (`pulumi preview`) |
| Execution | Apply from saved plan | Direct execution |
| Parallelism | Configurable via `-parallelism` | Automatic based on dependencies |

**Impact**: Pulumi HCL executes resource operations in parallel based on the dependency graph. The execution model is designed for the Pulumi engine's approach to infrastructure management.

### Provider Configuration

Pulumi HCL uses Pulumi providers (including Terraform-bridged providers) rather than native Terraform providers.

```hcl
terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"  # Use pulumi/ prefix
      version = ">= 6.0"
    }
  }
}
```

**Type name resolution**:
- Terraform-style names (e.g., `aws_instance`) are automatically mapped to Pulumi tokens for bridged providers
- For non-bridged providers, use Pulumi-style type names directly (e.g., `aws:ec2/instance:Instance`)

### Attribute Name Conversion

Terraform uses `snake_case` for attribute names, while Pulumi uses `camelCase`. Automatic conversion is applied:

```hcl
resource "aws_instance" "web" {
  instance_type = "t3.micro"  # Sent to Pulumi as "instanceType"
}

output "ip" {
  value = aws_instance.web.public_ip  # Received from Pulumi as "publicIp"
}
```

This conversion is transparent in your HCL code—always use `snake_case` as you would in Terraform.

### Lifecycle Block Differences

The `lifecycle` block is supported with the following behavioral differences:

| Lifecycle Option | Terraform Behavior | Pulumi HCL Behavior |
|------------------|-------------------|---------------------|
| `prevent_destroy` | Prevents destruction | Maps to Pulumi's `protect` option |
| `create_before_destroy` | Creates replacement before destroying original | Parsed but not enforced (Pulumi handles replacement ordering) |
| `ignore_changes` | Ignores changes to specified attributes | Supported via `ignoreChanges` resource option |
| `replace_triggered_by` | Forces replacement when referenced resources change | Parsed but no special handling |
| `precondition` / `postcondition` | Validates conditions | Parsed but not enforced |

### Secrets Handling

Pulumi automatically wraps values marked as `sensitive` as Pulumi secrets:

```hcl
variable "database_password" {
  type      = string
  sensitive = true  # Automatically becomes a Pulumi secret
}

output "connection_string" {
  value     = "postgres://user:${var.database_password}@host/db"
  sensitive = true  # Output is marked as secret
}
```

### Resource Addressing

Resources are addressed differently in the underlying system:

| Aspect | Terraform | Pulumi HCL |
|--------|-----------|------------|
| Basic resource | `aws_instance.web` | `aws_instance.web` (URN: `urn:pulumi:stack::project::aws:ec2/instance:Instance::aws_instance.web`) |
| With count | `aws_instance.web[0]` | `aws_instance.web[0]` |
| With for_each | `aws_instance.web["key"]` | `aws_instance.web["key"]` |

HCL references work identically in your code, but the underlying Pulumi URNs differ from Terraform resource addresses.

## Current Limitations

These are features that are not yet implemented but may be added in future releases. They do not represent fundamental incompatibilities.

### Module Support

Module blocks are parsed but not executed:

```hcl
# NOT YET SUPPORTED
module "vpc" {
  source = "./modules/vpc"

  cidr_block = "10.0.0.0/16"
}

# Will result in error: "module support not yet implemented"
```

**Workaround**: Inline the module resources directly, or refactor to use Pulumi components in a different Pulumi language (TypeScript, Python, etc.).

### Provisioners

Provisioner blocks are parsed but not executed at runtime:

```hcl
resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  # PARSED BUT NOT EXECUTED
  provisioner "remote-exec" {
    inline = ["sudo apt-get update"]
  }

  # PARSED BUT NOT EXECUTED
  provisioner "local-exec" {
    command = "echo ${self.private_ip}"
  }
}
```

**Workaround**: Use the [Pulumi Command provider](https://www.pulumi.com/registry/packages/command/) for executing commands:

```hcl
resource "command_remote_Command" "install" {
  connection {
    host = aws_instance.web.public_ip
  }
  create = "sudo apt-get update"
}
```

### Variable Configuration Integration

Variables currently only use default values from HCL. Pulumi stack configuration values are not yet integrated:

```hcl
variable "region" {
  type    = string
  default = "us-west-2"  # Only this default is used
}
```

**Workaround**: Ensure all variables have appropriate defaults, or set values using environment variables (`TF_VAR_region`).

### Variable Validation

Variable validation blocks are parsed but not enforced:

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

### State Migration Blocks

The following blocks are not supported:

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

**Workaround**: Use `pulumi state rename` for moves, and `pulumi import` for importing existing resources.

### Functions Not Implemented

The following function is not yet available:

| Function | Status | Workaround |
|----------|--------|------------|
| `rsadecrypt()` | Not implemented | Use external tooling to decrypt values before passing to Pulumi |

## Migration Guide

When migrating from Terraform to Pulumi HCL:

1. **State**: Existing Terraform state is not compatible. Either:
   - Import existing resources using `pulumi import`
   - Start fresh and recreate resources

2. **Modules**: Inline module resources or convert to Pulumi components

3. **Provisioners**: Replace with Pulumi Command provider resources

4. **Backend configuration**: Remove or comment out `backend` and `cloud` blocks

5. **Provider sources**: Update to use `pulumi/` prefixed provider sources:
   ```hcl
   # Before (Terraform)
   source = "hashicorp/aws"

   # After (Pulumi HCL)
   source = "pulumi/aws"
   ```

6. **Variables**: Ensure all variables have defaults until stack configuration integration is complete

## Feature Comparison Summary

| Feature | Terraform | Pulumi HCL | Notes |
|---------|:---------:|:----------:|-------|
| Resources | Yes | Yes | Full support |
| Data sources | Yes | Yes | Full support |
| Variables | Yes | Yes | Defaults only (config integration pending) |
| Locals | Yes | Yes | Full support |
| Outputs | Yes | Yes | Full support |
| Providers | Yes | Yes | Pulumi providers |
| Modules | Yes | No | Not yet implemented |
| Provisioners | Yes | No | Use Command provider |
| Workspaces | Yes | N/A | Use Pulumi stacks |
| State backends | Yes | N/A | Use Pulumi state management |
| `moved` blocks | Yes | No | Use `pulumi state` |
| `import` blocks | Yes | No | Use `pulumi import` |
| `count` | Yes | Yes | Full support |
| `for_each` | Yes | Yes | Full support |
| `depends_on` | Yes | Yes | Full support |
| `lifecycle` | Yes | Partial | Some options mapped differently |
| Functions | Yes | Mostly | 80+ functions, `rsadecrypt` pending |

## Getting Help

If you encounter compatibility issues not covered in this document:

1. Check the [Pulumi HCL GitHub repository](https://github.com/pulumi/pulumi-language-hcl) for known issues
2. File an issue with a minimal reproduction case
3. Join the [Pulumi Community Slack](https://slack.pulumi.com/) for community support
