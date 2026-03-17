# Pulumi HCL Language Plugin

A Pulumi language plugin that enables writing infrastructure as code using Terraform-compatible HCL syntax, powered by the Pulumi engine.

## Overview

This plugin allows you to use familiar Terraform/HCL syntax while leveraging Pulumi's state management, secrets handling, and cloud platform. It parses HCL files and translates them to Pulumi resource registrations at runtime.

```hcl
# main.hcl
resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-unique-bucket-name"

  tags = {
    Environment = "dev"
    ManagedBy   = "Pulumi"
  }
}

output "bucket_arn" {
  value = aws_s3_bucket.my_bucket.arn
}
```

## Installation

Install the plugin onto your path:

```bash
go install github.com/pulumi/pulumi-language-hcl/cmd/pulumi-language-hcl # for the language
go install github.com/pulumi/pulumi-language-hcl/cmd/pulumi-converter-hcl # for the converter
```

## Usage

1. Create a `Pulumi.yaml` with `runtime: hcl`:

```yaml
name: my-project
runtime: hcl
description: My HCL project
```

2. Create HCL files (`.hcl` extension):

```hcl
# main.hcl
resource "random_pet" "my_pet" {
  length = 2
}

output "pet_name" {
  value = random_pet.my_pet.id
}
```

3. Run Pulumi commands as usual:

```bash
pulumi up
```

## Supported Features

### Resources

```hcl
resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  tags = {
    Name = "web-server"
  }
}
```

### Data Sources

```hcl
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-*-amd64-server-*"]
  }
}
```

### Variables

```hcl
variable "region" {
  type        = string
  default     = "us-west-2"
  description = "AWS region"
}

variable "instance_count" {
  type    = number
  default = 1
}
```

### Locals

```hcl
locals {
  common_tags = {
    Environment = "dev"
    Project     = "my-project"
  }

  name_prefix = "myapp-${var.environment}"
}
```

### Outputs

```hcl
output "instance_ip" {
  value       = aws_instance.web.public_ip
  description = "Public IP of the instance"
}
```

### Meta-Arguments

```hcl
resource "aws_instance" "web" {
  count = var.instance_count
  # or
  for_each = var.instances

  ami           = data.aws_ami.ubuntu.id
  instance_type = each.value.type

  depends_on = [aws_security_group.allow_http]

  lifecycle {
    create_before_destroy = true
    ignore_changes        = [tags["Timestamp"]]
  }

  timeouts {
    create = "60m"
    update = "30m"
    delete = "2h"
  }
}
```

### Modules

```hcl
# Local module
module "vpc" {
  source = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}

# Git source
module "eks" {
  source = "git::https://github.com/org/terraform-aws-eks.git?ref=v1.0.0"
}

# Terraform Registry
module "rds" {
  source  = "terraform-aws-modules/rds/aws"
  version = "6.0.0"
}

# GitHub shorthand
module "lambda" {
  source = "github.com/org/terraform-aws-lambda"
}
```

Modules map to Pulumi component resources. All source types are supported: local paths, Git URLs, GitHub/BitBucket shorthand, Terraform Registry, and HTTP archives.

### Provisioners

```hcl
resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  provisioner "local-exec" {
    command = "echo ${self.public_ip} >> hosts.txt"
  }

  provisioner "remote-exec" {
    inline = [
      "sudo apt-get update",
      "sudo apt-get install -y nginx"
    ]
  }

  provisioner "file" {
    source      = "config.txt"
    destination = "/tmp/config.txt"
  }

  connection {
    type        = "ssh"
    user        = "ubuntu"
    private_key = file("~/.ssh/id_rsa")
    host        = self.public_ip
  }
}
```

Provisioners map to the [Pulumi Command provider](https://www.pulumi.com/registry/packages/command/):
- `local-exec` → `command:local:Command`
- `remote-exec` → `command:remote:Command`
- `file` → `command:remote:CopyToRemote`

WinRM connections are not supported — SSH only.

### Moved and Import Blocks

```hcl
# Rename a resource without recreating it
moved {
  from = aws_instance.old_name
  to   = aws_instance.new_name
}

# Import an existing resource
import {
  to = aws_instance.web
  id = "i-1234567890abcdef0"
}
```

### Provider Configuration

```hcl
terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"
      version = ">= 6.0"
    }
  }
}

provider "aws" {
  region = var.region
}
```

The `backend`, `cloud`, and `required_version` fields in `terraform` blocks are parsed but ignored (Pulumi manages state and versioning independently).

## Design Overview

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Pulumi Engine                             │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐ │
│  │   CLI       │  │ State Mgmt   │  │   Provider Plugins      │ │
│  └─────────────┘  └──────────────┘  └─────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC (LanguageRuntime)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                   pulumi-language-hcl                            │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Server (pkg/server)                                         ││
│  │  - LanguageRuntimeServer gRPC implementation                ││
│  │  - GetRequiredPlugins, Run, GetProgramDependencies          ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Parser (pkg/hcl/parser)                                     ││
│  │  - Uses hashicorp/hcl/v2 (MPL licensed)                     ││
│  │  - Parses *.hcl files into AST                              ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ AST (pkg/hcl/ast)                                           ││
│  │  - Config, Resource, Variable, Local, Output, Provider      ││
│  │  - Terraform-compatible block structures                    ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Graph (pkg/hcl/graph)                                       ││
│  │  - Dependency extraction from HCL expressions               ││
│  │  - Topological sort for execution order                     ││
│  │  - Parallel execution scheduler                             ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Evaluator (pkg/hcl/eval)                                    ││
│  │  - HCL expression evaluation                                ││
│  │  - Terraform-compatible function library                    ││
│  │  - Variable/resource reference resolution                   ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Packages (pkg/hcl/packages)                                 ││
│  │  - Pulumi provider schema loading                           ││
│  │  - TF resource type → Pulumi token mapping                  ││
│  │  - Cached provider info for fast startup                    ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Run Engine (pkg/hcl/run)                                    ││
│  │  - Orchestrates execution                                   ││
│  │  - Registers resources with Pulumi                          ││
│  │  - Handles count/for_each expansion                         ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Transform (pkg/hcl/transform)                               ││
│  │  - cty.Value ↔ Pulumi PropertyValue conversion              ││
│  │  - camelCase ↔ snake_case name mapping                      ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

### Execution Flow

1. **Parse**: HCL files are parsed using `hashicorp/hcl/v2` into an AST
2. **Graph**: Dependencies are extracted and a DAG is built
3. **Execute**: Nodes are processed in parallel where dependencies allow:
   - Variables → set in evaluation context
   - Locals → evaluated and stored
   - Resources/Data Sources → registered with Pulumi (parallel)
   - Outputs → collected and registered on stack

### Type Resolution

Pulumi HCL supports Terraform-style resource type names:

```hcl
# Terraform-style
resource "aws_ec2_instance" "web" { }      # → aws:ec2/instance:Instance
```

Type resolution is conducted with the following algorithm:

1. The provider package name is extracted from the first underscore-delimited segment of the HCL type (e.g. `aws` from `aws_s3_bucket`). If `required_providers` is specified, the longest matching provider prefix is used to resolve ambiguity.
2. The provider's schema is loaded from the Pulumi registry.
3. Each resource in the provider schema has its module path and resource name lowercased and joined, with all underscores and slashes stripped, forming a lookup key.
4. The remaining portion of the HCL type (after the provider prefix) is compared against these keys (also with underscores stripped) to find the matching resource.

For example, `aws_ec2_instance` → provider `aws`, lookup key `ec2instance` → matches `aws:ec2/instance:Instance` in the AWS schema.

### Name Conversion

Pulumi HCL expects `snake_case` properties. The plugin ensures that the engine sees Pulumi's `camelCase` property
names. Map keys are not translated.

## Multi-Language Components

HCL modules can be published as reusable Pulumi components consumable from any language. See [docs/mlc.md](docs/mlc.md) for details on authoring MLCs with the `pulumi { component { ... } package { ... } }` syntax.

## Terraform Compatibility

This plugin supports the majority of Terraform's HCL syntax. For detailed compatibility information and known limitations, see [docs/terraform-compatibility.md](docs/terraform-compatibility.md).

### Supported

- `resource` blocks with all meta-arguments (`count`, `for_each`, `depends_on`, `lifecycle`, `timeouts`)
- `data` source blocks
- `variable` blocks with defaults and types
- `locals` blocks
- `output` blocks
- `provider` blocks
- `terraform.required_providers` block
- `module` blocks (local, Git, Terraform Registry, HTTP sources)
- `provisioner` blocks (`local-exec`, `remote-exec`, `file`)
- `moved` blocks (map to Pulumi aliases)
- `import` blocks (map to Pulumi import option)
- Most Terraform built-in functions
- Resource and data source references
- Splat expressions (`resource.name[*].attr`)

### Not Supported

- `replace_triggered_by` lifecycle option (different semantics from Pulumi's `replaceOnChanges`)
- `dynamic` blocks (dynamic block generation is not implemented)
- `List<Object>` empty vs null distinction: HCL block syntax cannot distinguish between an empty and null `List<Object>`, which is a known incompatibility with some Pulumi programs

### Pulumi-Specific Extensions

```hcl
# Stack references
data "pulumi_stack_reference" "network" {
  name = "myorg/networking/prod"
}

output "vpc_id" {
  value = data.pulumi_stack_reference.network.outputs["vpc_id"]
}
```

```hcl
# Method calls on resources
call "aws_s3_bucket.my_bucket" "getObject" {
  key = "config.json"
}
```

The `call` block invokes a method on an existing resource. The first label is `resourceType.resourceName` and the second is the method name. Results are referenced as `call.<resource>.<method>.<output>`.

Two built-in functions provide access to a resource's Pulumi identity at runtime:
- `pulumiResourceName(resource)` — returns the logical name from the resource's URN
- `pulumiResourceType(resource)` — returns the type token from the resource's URN

## Development

### Building

```bash
make build    # Outputs to bin/
```

### Testing

```bash
make test
# or
go test ./...
```

### Running Examples

```bash
cd examples/simple
pulumi up
```

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.

Note: This project uses `github.com/hashicorp/hcl/v2` which is licensed under MPL 2.0.
