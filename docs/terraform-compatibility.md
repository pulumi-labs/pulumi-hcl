# Terraform HCL Compatibility

Pulumi HCL lets you write infrastructure using Terraform-compatible HCL syntax while running on the Pulumi engine. This document covers what works, what's different, and what's not yet implemented.

## Supported Features

The following HCL constructs work with identical behavior to Terraform:

### Core Blocks

| Block | Notes |
|-------|-------|
| `resource` | Full support including nested blocks |
| `data` | Data sources invoked via Pulumi provider functions |
| `variable` | Type constraints, defaults, nullable, sensitive, validation |
| `locals` | Full expression interpolation |
| `output` | value, description, sensitive, depends_on, precondition |
| `provider` | Including alias support |
| `terraform.required_providers` | Provider version constraints |
| `module` | Local modules with count/for_each support |

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
| `create_before_destroy` | [`deleteBeforeReplace`](https://www.pulumi.com/docs/iac/concepts/resources/options/deletebeforereplace/) (inverted) |
| `precondition` | Evaluated before resource creation |
| `postcondition` | Evaluated after resource creation (with `self` reference) |

### Variable Configuration

Variables support the full precedence chain:

1. Environment variables (`TF_VAR_<name>`) - highest priority
2. Pulumi stack configuration (`pulumi config set`) - via `projectName:varName`
3. Default values in variable blocks - lowest priority

```hcl
variable "region" {
  type    = string
  default = "us-west-2"
}
# Set via: TF_VAR_region=us-east-1 or pulumi config set myproject:region us-east-1
```

### Variable Validation

Validation blocks are fully enforced:

```hcl
variable "instance_type" {
  type    = string
  default = "t3.micro"

  validation {
    condition     = startswith(var.instance_type, "t3.")
    error_message = "Must be a t3 instance type."
  }
}
# Fails with custom error message if validation fails
```

### Modules

Local modules are fully supported and map to Pulumi component resources:

```hcl
module "vpc" {
  source     = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}

output "vpc_id" {
  value = module.vpc.vpc_id
}
```

Modules support:
- `count` and `for_each` for multiple instances
- Input variables passed from the parent
- Output values accessible as `module.<name>.<output>`
- Nested modules (modules within modules)
- Cycle detection to prevent infinite recursion

**Limitations**:
- Only local paths (`./path`, `../path`) are supported
- Registry modules and Git sources are not yet implemented

### Provisioners

Provisioner blocks map to the [Pulumi Command provider](https://www.pulumi.com/registry/packages/command/):

| Terraform | Pulumi |
|-----------|--------|
| `local-exec` | `command:local:Command` |
| `remote-exec` | `command:remote:Command` |
| `file` | `command:remote:CopyToRemote` |

```hcl
resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t3.micro"

  provisioner "local-exec" {
    command     = "echo ${self.id} >> instances.txt"
    working_dir = "/tmp"
  }

  provisioner "remote-exec" {
    inline = ["sudo apt-get update", "sudo apt-get install -y nginx"]

    connection {
      host = self.public_ip
      user = "ubuntu"
    }
  }
}
```

Provisioner features:
- `self` references to parent resource outputs
- `when = "destroy"` for cleanup commands
- `on_failure = "continue"` for error handling
- Connection blocks for SSH configuration
- Sequential execution with dependency chaining

**Limitations**:
- WinRM connections are not supported (SSH only)
- `script` and `scripts` attributes execute via `sh` on the remote host

### Expressions

| Feature | Notes |
|---------|-------|
| String interpolation | `"${var.name}-suffix"` |
| Splat expressions | `resource.name[*].attr` |
| Resource references | `aws_instance.web.id`, `data.aws_ami.ubuntu.id` |
| Module references | `module.vpc.vpc_id` |
| Complex types | Lists, maps, sets, objects, tuples |
| Built-in functions | 80+ Terraform functions including `rsadecrypt` |

### Sensitive Values

Values marked `sensitive` become Pulumi secrets, providing encryption at rest:

```hcl
variable "database_password" {
  type      = string
  sensitive = true  # Encrypted in Pulumi state
}
```

## Platform Differences

These are expected differences when using Pulumi instead of Terraform. They reflect fundamental differences in how the engines work.

### Resource Replacement Order

Pulumi and Terraform have opposite default behaviors for resource replacement:

| Engine | Default Behavior | Override |
|--------|------------------|----------|
| Terraform | Delete old, then create new | `create_before_destroy = true` |
| Pulumi | Create new, then delete old | `deleteBeforeReplace = true` |

Pulumi HCL uses **Pulumi's default** (create-then-delete) for consistency with other Pulumi languages. This is generally safer as it minimizes downtime, but may cause issues with resources that have unique constraints (DNS names, ports, etc.).

To get Terraform's default behavior, explicitly set:

```hcl
resource "aws_instance" "web" {
  # ...

  lifecycle {
    # This is a no-op in Terraform but enables delete-then-create in Pulumi
    create_before_destroy = false
  }
}
```

**Note**: Setting `create_before_destroy = false` explicitly will trigger Pulumi's `deleteBeforeReplace = true`, matching Terraform's default behavior.

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

### Remote Module Sources

Only local module paths are currently supported:

```hcl
module "vpc" {
  source = "./modules/vpc"  # Supported
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"  # Not yet supported
  version = "19.0"
}
# Error: registry module sources not yet supported
```

**Workaround**: Clone or vendor remote modules locally.

### Timeouts Block

Resource timeout configuration is parsed but not yet enforced:

```hcl
resource "aws_instance" "web" {
  timeouts {
    create = "60m"
    delete = "2h"
  }
}
# Timeouts are ignored
```

## Quick Reference

| Feature | Status | Notes |
|---------|:------:|-------|
| Resources | Yes | |
| Data sources | Yes | |
| Variables | Yes | Full config integration |
| Variable validation | Yes | |
| Locals | Yes | |
| Outputs | Yes | Including preconditions |
| Providers | Yes | Use `pulumi/` source prefix |
| `count` / `for_each` | Yes | |
| `depends_on` | Yes | |
| `prevent_destroy` | Yes | Maps to `protect` |
| `ignore_changes` | Yes | Maps to `ignoreChanges` |
| `create_before_destroy` | Yes | Maps to `deleteBeforeReplace` |
| `precondition` / `postcondition` | Yes | |
| Modules (local) | Yes | Maps to component resources |
| Modules (remote) | Pending | Local paths only |
| Provisioners | Yes | Maps to Command provider |
| `replace_triggered_by` | No | No direct Pulumi equivalent |
| `moved` / `import` blocks | No | Use CLI commands |
| Functions | Yes | 80+ including `rsadecrypt` |
| Timeouts | Pending | Parsed but not enforced |

## Getting Help

If you encounter compatibility issues:

1. Check the [GitHub repository](https://github.com/pulumi/pulumi-language-hcl) for known issues
2. File an issue with a minimal reproduction
3. Join the [Pulumi Community Slack](https://slack.pulumi.com/)
