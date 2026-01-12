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

Build and install the plugin:

```bash
make build
mkdir -p ~/.pulumi/plugins/language-hcl-v0.0.1
cp bin/pulumi-language-hcl ~/.pulumi/plugins/language-hcl-v0.0.1/
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

Modules map to Pulumi component resources. All source types are supported: local paths, Git URLs, GitHub/BitBucket shorthand, Terraform Registry, and HTTP archives. Remote modules are cached in `~/.pulumi/modules/`.

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
3. **Sort**: Topological sort determines valid execution order
4. **Execute**: Nodes are processed in parallel where dependencies allow:
   - Variables → set in evaluation context
   - Locals → evaluated and stored
   - Resources/Data Sources → registered with Pulumi (parallel)
   - Outputs → collected and registered on stack

### Type Resolution

The plugin supports both Terraform-style and Pulumi-style resource type names:

```hcl
# Terraform-style (looked up via provider bridge mapping)
resource "aws_instance" "web" { }      # → aws:ec2/instance:Instance

# Pulumi-style (used directly)
resource "aws:ec2/instance:Instance" "web" { }
```

For Terraform-bridged providers (AWS, Azure, GCP, etc.), type resolution uses the provider's `-get-provider-info` output which maps TF types to Pulumi tokens.

### Caching

Provider information is cached in `~/.pulumi/plugins/resource-{provider}-v{version}/pulumi-hcl.cache`:

```json
{
  "name": "aws",
  "version": "6.0.0",
  "isBridged": true,
  "resources": {
    "aws_instance": "aws:ec2/instance:Instance",
    "aws_s3_bucket": "aws:s3/bucket:Bucket"
  },
  "dataSources": {
    "aws_ami": "aws:ec2/getAmi:getAmi"
  },
  "resourceTokens": ["aws:ec2/instance:Instance", ...],
  "functionTokens": ["aws:ec2/getAmi:getAmi", ...]
}
```

This avoids expensive provider invocations and schema parsing on subsequent runs.

### Parallel Execution

Independent resources are processed in parallel:

```
Variables (sequential)
    │
    ▼
Locals (sequential)
    │
    ▼
Resources & Data Sources (parallel with dependency tracking)
    │
    ├── data.aws_ami ─────────────┐
    │                             │
    ├── aws_security_group ───────┼──► aws_instance (waits for both)
    │                             │
    └─────────────────────────────┘
```

### Name Conversion

Terraform uses `snake_case` for attribute names, while Pulumi uses `camelCase`. The plugin automatically converts:

- Input: `instance_type` → `instanceType` (when sending to Pulumi)
- Output: `publicIp` → `public_ip` (when reading from Pulumi)

## Terraform Compatibility

This plugin provides full compatibility with Terraform's HCL syntax. For detailed compatibility information, see [docs/terraform-compatibility.md](docs/terraform-compatibility.md).

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

## Project Structure

```
pulumi-language-hcl/
├── docs/
│   └── terraform-compatibility.md  # Detailed compatibility documentation
├── examples/
│   ├── simple/                     # Basic random_pet example
│   └── aws-webserver/              # AWS EC2 example
├── pkg/
│   ├── hcl/
│   │   ├── ast/                    # AST types
│   │   ├── eval/                   # Expression evaluator
│   │   ├── graph/                  # Dependency graph
│   │   ├── modules/                # Module loading (local/remote)
│   │   ├── packages/               # Provider schema loading
│   │   ├── parser/                 # HCL parser
│   │   ├── run/                    # Execution engine
│   │   ├── schema/                 # Schema generation
│   │   └── transform/              # Type conversions
│   ├── server/                     # gRPC server
│   └── version/                    # Version info
├── go.mod
├── Makefile
└── README.md
```

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
