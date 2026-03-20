# Pulumi HCL Language Reference

Pulumi HCL lets you write Pulumi programs using Terraform-compatible HCL syntax. You get familiar HCL blocks, expressions, and functions while using Pulumi's state management, secrets, and deployment engine.

## Table of Contents

- [Overview](#overview)
- [Top-Level Blocks](#top-level-blocks)
- [Variables](#variables)
- [Resources](#resources)
- [Resource Options](#resource-options)
- [Data Sources](#data-sources)
- [Providers](#providers)
- [Outputs](#outputs)
- [Locals](#locals)
- [Modules](#modules)
- [Call Blocks](#call-blocks)
- [Moved and Import Blocks](#moved-and-import-blocks)
- [Expressions](#expressions)
- [Built-in Functions](#built-in-functions)
- [Stack References](#stack-references)
- [Pulumi Block](#pulumi-block)
- [Terraform Compatibility](#terraform-compatibility)

## Overview

A Pulumi HCL program consists of one or more `.hcl` files in a directory with a `Pulumi.yaml` specifying `runtime: hcl`:

```yaml
name: my-project
runtime: hcl
```

HCL files declare infrastructure using blocks for [resources](#resources), [data sources](#data-sources), [variables](#variables), [locals](#locals), [outputs](#outputs), [providers](#providers), [modules](#modules), and Pulumi-specific constructs like [call blocks](#call-blocks). Programs also support [moved and import blocks](#moved-and-import-blocks) for resource lifecycle management, [stack references](#stack-references) for cross-stack access, and a [pulumi block](#pulumi-block) for version constraints and multi-language component declarations.

The full set of [expressions](#expressions) and [built-in functions](#built-in-functions) from Terraform's HCL is available, along with Pulumi-specific asset/archive functions. See [Terraform Compatibility](#terraform-compatibility) for the small number of differences.

## Top-Level Blocks

| Block       | Purpose                                           |
|-------------|---------------------------------------------------|
| `variable`  | Declare input variables                           |
| `resource`  | Manage cloud resources                            |
| `data`      | Read external data via provider invocations       |
| `provider`  | Configure provider instances                      |
| `output`    | Export values from the stack                      |
| `locals`    | Define reusable intermediate values               |
| `module`    | Invoke local or remote modules as components      |
| `call`      | Invoke methods on resources                       |
| `moved`     | Rename resources without recreation               |
| `import`    | Import existing cloud resources                   |
| `terraform` | Declare required providers (and ignored settings) |
| `pulumi`    | Version constraints and component declarations    |

## Variables

Variables declare input values for the program. Values are set via Pulumi's configuration system.

```hcl
variable "region" {
  type        = string
  default     = "us-west-2"
  description = "AWS region to deploy into"
}

variable "instance_count" {
  type    = number
  default = 1
}

variable "enable_monitoring" {
  type      = bool
  sensitive = true
}

variable "name" {
  type = string

  validation {
    condition     = length(var.name) > 0
    error_message = "Name must not be empty."
  }
}
```

### Attributes

| Attribute     | Type       | Required | Description                                                                                        |
|---------------|------------|----------|----------------------------------------------------------------------------------------------------|
| `type`        | type expr  | No       | Type constraint (e.g., `string`, `number`, `bool`, `list(string)`, `map(number)`, `object({...})`) |
| `default`     | expression | No       | Default value when not configured                                                                  |
| `description` | string     | No       | Human-readable description                                                                         |
| `sensitive`   | bool       | No       | When `true`, the value becomes a Pulumi secret                                                     |
| `nullable`    | bool       | No       | When `false`, rejects null values (default: `true`)                                                |
| `validation`  | block      | No       | One or more validation rules (see below)                                                           |

### Validation Blocks

Each `validation` block has:

| Attribute       | Type       | Required | Description                                   |
|-----------------|------------|----------|-----------------------------------------------|
| `condition`     | expression | Yes      | Expression that must evaluate to `true`       |
| `error_message` | expression | Yes      | Error message shown when condition is `false` |

### Setting Variable Values

Variables are set through Pulumi's config system, in priority order:

1. **Environment variables**: `TF_VAR_name=value` (highest priority)
2. **Stack config**: `pulumi config set <project>:<varName> <value>`
3. **Default values** in `variable` blocks (lowest priority)

Reference variables in expressions as `var.<name>`:

```hcl
resource "aws_instance" "web" {
  instance_type = var.instance_type
}
```

## Resources

Resources declare managed infrastructure.

```hcl
resource "aws_instance" "web" {
  ami           = "ami-12345678"
  instance_type = "t3.micro"

  tags = {
    Name = "web-server"
  }
}
```

The first label is the resource type (Terraform-style, e.g. `aws_instance`). The second label is the logical name. The body contains the resource's input properties.

### Meta-Arguments

| Argument     | Type       | Description                                            |
|--------------|------------|--------------------------------------------------------|
| `count`      | number     | Create multiple instances indexed by `count.index`     |
| `for_each`   | map or set | Create instances keyed by `each.key` with `each.value` |
| `depends_on` | list       | Explicit dependencies on other resources               |
| `provider`   | reference  | Specific provider configuration to use                 |
| `providers`  | list       | Explicit provider resources (component resources only) |

### Lifecycle Block

```hcl
resource "aws_instance" "web" {
  # ...

  lifecycle {
    create_before_destroy = true
    prevent_destroy       = true
    ignore_changes        = [tags]
  }
}
```

| Attribute               | Type      | Description                                              |
|-------------------------|-----------|----------------------------------------------------------|
| `create_before_destroy` | bool      | When `false`, deletes before replacing (Pulumi creates first by default) |
| `prevent_destroy`       | bool      | Protect resource from accidental deletion (maps to Pulumi `protect`) |
| `ignore_changes`        | list      | Property paths to exclude from diff detection            |

### Timeouts Block

```hcl
resource "aws_instance" "web" {
  # ...

  timeouts {
    create = "60m"
    update = "30m"
    delete = "2h"
  }
}
```

| Attribute | Type   | Description                   |
|-----------|--------|-------------------------------|
| `create`  | string | Timeout for create operations |
| `read`    | string | Timeout for read operations   |
| `update`  | string | Timeout for update operations |
| `delete`  | string | Timeout for delete operations |

### Preconditions and Postconditions

Preconditions and postconditions are nested inside the `lifecycle` block:

```hcl
resource "aws_instance" "web" {
  # ...

  lifecycle {
    precondition {
      condition     = var.instance_type != ""
      error_message = "Instance type must be specified."
    }

    postcondition {
      condition     = self.public_ip != ""
      error_message = "Instance must have a public IP."
    }
  }
}
```

### Provisioners

Provisioners run commands after resource creation. They map to the [Pulumi Command provider](https://www.pulumi.com/registry/packages/command/).

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
      "sudo apt-get install -y nginx",
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

| Provisioner Type | Pulumi Equivalent             |
|------------------|-------------------------------|
| `local-exec`     | `command:local:Command`       |
| `remote-exec`    | `command:remote:Command`      |
| `file`           | `command:remote:CopyToRemote` |

Provisioner options:

| Attribute    | Values                  | Description               |
|--------------|-------------------------|---------------------------|
| `when`       | `"create"`, `"destroy"` | When the provisioner runs |
| `on_failure` | `"continue"`, `"fail"`  | Behavior on failure       |

Connection blocks support SSH only (WinRM is not supported). The `self` reference provides access to the parent resource's attributes.

### Dynamic Blocks

Dynamic blocks generate repeated nested blocks from a collection:

```hcl
resource "aws_security_group" "web" {
  name = "web-sg"

  dynamic "ingress" {
    for_each = var.ingress_rules
    content {
      from_port   = ingress.value.from_port
      to_port     = ingress.value.to_port
      protocol    = ingress.value.protocol
      cidr_blocks = ingress.value.cidr_blocks
    }
  }
}
```

The label on the `dynamic` block becomes the iterator variable name. Inside the `content` block, `<label>.value` refers to the current element and `<label>.key` refers to its key or index.

### Referencing Resources

Resources are referenced by `<type>.<name>`:

```hcl
output "instance_id" {
  value = aws_instance.web.id
}
```

When using `count`, instances are indexed: `aws_instance.web[0].id` or `aws_instance.web[*].id` (splat).

When using `for_each`, instances are keyed: `aws_instance.web["key"].id`.

## Resource Options

These are Pulumi-specific meta-arguments available on resource blocks.

```hcl
resource "aws_instance" "web" {
  # ...

  parent                     = module.my_component
  additional_secret_outputs  = ["password"]
  retain_on_delete           = true
  deleted_with               = aws_vpc.main
  replace_on_changes         = ["ami"]
  replace_with               = [aws_instance.replacement]
  hide_diffs                 = ["user_data"]
  replacement_trigger        = var.force_replace
  import_id                  = "i-1234567890abcdef0"
  aliases                    = ["old-name"]
  version                    = "6.0.0"
  plugin_download_url        = "https://example.com/plugins"
}
```

| Attribute                   | Type         | Description                                                  |
|-----------------------------|--------------|--------------------------------------------------------------|
| `parent`                    | reference    | Parent resource for component hierarchy                      |
| `additional_secret_outputs` | list(string) | Output properties to encrypt in state                        |
| `retain_on_delete`          | bool         | Keep the cloud resource when removed from the program        |
| `deleted_with`              | reference    | Cascade deletion when the referenced resource is deleted     |
| `replace_with`              | list         | Resources whose replacement triggers replacement of this one |
| `hide_diffs`                | list(string) | Property paths whose diffs should not be displayed           |
| `replace_on_changes`        | list(string) | Property paths that force replacement when changed           |
| `replacement_trigger`       | expression   | Expression whose change triggers replacement                 |
| `import_id`                 | string       | Cloud resource ID to import                                  |
| `aliases`                   | list         | Alternative names for this resource (used during renames)    |
| `version`                   | string       | Provider plugin version                                      |
| `plugin_download_url`       | string       | URL to download the provider plugin from                     |

Provider resources (`resource "pulumi_providers_*"`) additionally accept:

| Attribute          | Type       | Description                                      |
|--------------------|------------|--------------------------------------------------|
| `env_var_mappings` | expression | Environment variable remappings for the provider |

## Data Sources

Data sources read information from providers via invocations.

```hcl
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-*-amd64-server-*"]
  }
}

output "ami_id" {
  value = data.aws_ami.ubuntu.id
}
```

Data sources use the `data` block with the same type naming as resources. Results are referenced as `data.<type>.<name>.<attribute>`.

Data sources support the same meta-arguments as resources: `count`, `for_each`, `depends_on`, and `provider`.

## Providers

Providers supply the implementation for resources and data sources.

### Required Providers

Declare provider requirements in the `terraform` block:

```hcl
terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"
      version = ">= 6.0"
    }
    random = {
      source  = "pulumi/random"
      version = "4.16.0"
    }
  }
}
```

Provider sources must use the `pulumi/` namespace (not `hashicorp/`).

### Provider Configuration

Configure providers with `provider` blocks:

```hcl
provider "aws" {
  region = "us-west-2"
}
```

### Multiple Provider Configurations

Use `alias` to create multiple configurations of the same provider:

```hcl
provider "aws" {
  region = "us-west-2"
}

provider "aws" {
  alias  = "east"
  region = "us-east-1"
}

resource "aws_instance" "web" {
  provider = aws.east
  # ...
}
```

### Provider Resources

Providers can also be declared as resources for use with component resources:

```hcl
resource "pulumi_providers_aws" "explicit" {
  region = "us-west-2"
}

resource "aws_instance" "web" {
  providers = [pulumi_providers_aws.explicit]
  # ...
}
```

The `pulumi_providers_<name>` resource type creates an explicit provider instance. Pass it to component resources via the `providers` meta-argument.

## Outputs

Outputs export values from the stack.

```hcl
output "instance_ip" {
  value       = aws_instance.web.public_ip
  description = "Public IP of the web server"
}

output "db_password" {
  value     = random_password.db.result
  sensitive = true
}

output "vpc_id" {
  value      = aws_vpc.main.id
  depends_on = [aws_internet_gateway.gw]

  precondition {
    condition     = aws_vpc.main.id != ""
    error_message = "VPC must have an ID."
  }
}
```

| Attribute      | Type       | Required | Description                                     |
|----------------|------------|----------|-------------------------------------------------|
| `value`        | expression | Yes      | The value to export                             |
| `description`  | string     | No       | Human-readable description                      |
| `sensitive`    | bool       | No       | When `true`, the output becomes a Pulumi secret |
| `depends_on`   | list       | No       | Explicit dependencies                           |
| `precondition` | block      | No       | Validation checks before export                 |

## Locals

Locals define reusable intermediate values.

```hcl
locals {
  common_tags = {
    Environment = "dev"
    Project     = "my-project"
  }

  name_prefix = "myapp-${var.environment}"

  user_data = <<-EOF
    #!/bin/bash
    echo "Hello, World!" > index.html
    nohup python3 -m http.server 80 &
  EOF
}
```

Multiple `locals` blocks are allowed. Reference locals as `local.<name>`:

```hcl
resource "aws_instance" "web" {
  tags      = local.common_tags
  user_data = local.user_data
}
```

## Modules

Modules invoke reusable configurations as Pulumi component resources.

```hcl
module "vpc" {
  source     = "./modules/vpc"
  cidr_block = "10.0.0.0/16"
}

output "vpc_id" {
  value = module.vpc.vpc_id
}
```

### Module Sources

| Source Type           | Example                                                        |
|-----------------------|----------------------------------------------------------------|
| Local path            | `./modules/vpc`                                                |
| Git                   | `git::https://github.com/org/repo.git?ref=v1.0.0`              |
| Git with subdirectory | `git::https://github.com/org/repo.git//modules/vpc?ref=v1.0.0` |
| GitHub shorthand      | `github.com/org/repo`                                          |
| BitBucket shorthand   | `bitbucket.org/org/repo`                                       |
| Terraform Registry    | `terraform-aws-modules/vpc/aws`                                |
| HTTP archive          | `https://example.com/module.zip`                               |

Remote modules are cached in `~/.pulumi/modules/`.

### Module Meta-Arguments

| Argument     | Type       | Description                                    |
|--------------|------------|------------------------------------------------|
| `source`     | string     | Module source (required)                       |
| `version`    | string     | Version constraint (for registry modules)      |
| `count`      | number     | Create multiple module instances               |
| `for_each`   | map or set | Create keyed module instances                  |
| `depends_on` | list       | Explicit dependencies                          |
| `providers`  | map        | Provider configuration mappings for the module |

Module outputs are referenced as `module.<name>.<output_name>`.

## Call Blocks

Call blocks invoke methods on existing resources. This is a Pulumi-specific extension with no Terraform equivalent.

```hcl
resource "call_custom" "my_resource" {
  value = "hello"
}

call "my_resource" "provider_value" {
}

output "result" {
  value = call.my_resource.provider_value.result
}
```

The first label is the resource's logical name (matching a declared resource). The second label is the method name. The body contains arguments to the method.

Results are referenced as `call.<resource_name>.<method_name>.<attribute>`.

## Moved and Import Blocks

### Moved Blocks

Rename resources without recreating them. Maps to Pulumi's `aliases` resource option.

```hcl
moved {
  from = aws_instance.old_name
  to   = aws_instance.new_name
}
```

| Attribute | Type      | Required | Description               |
|-----------|-----------|----------|---------------------------|
| `from`    | reference | Yes      | Original resource address |
| `to`      | reference | Yes      | New resource address      |

### Import Blocks

Import existing cloud resources into Pulumi state.

```hcl
import {
  to       = aws_instance.web
  id       = "i-1234567890abcdef0"
  provider = aws.east
}
```

| Attribute  | Type      | Required | Description                   |
|------------|-----------|----------|-------------------------------|
| `to`       | reference | Yes      | Target resource address       |
| `id`       | string    | Yes      | Cloud resource ID to import   |
| `provider` | reference | No       | Provider configuration to use |

## Expressions

Pulumi HCL supports the full HCL expression language.

### Literals

```hcl
"hello"           # string
42                # number
3.14              # number
true              # bool
null              # null
["a", "b", "c"]   # list
{key = "value"}   # map
```

### String Interpolation

```hcl
"Hello, ${var.name}!"
"prefix-${local.env}-suffix"
```

### Heredocs

```hcl
<<-EOF
  multi-line
  string content
EOF
```

### References

| Reference                | Description                        |
|--------------------------|------------------------------------|
| `var.<name>`             | Input variable                     |
| `local.<name>`           | Local value                        |
| `<type>.<name>`          | Resource attribute                 |
| `<type>.<name>[<index>]` | Counted resource instance          |
| `<type>.<name>["<key>"]` | For-each resource instance         |
| `data.<type>.<name>`     | Data source attribute              |
| `module.<name>`          | Module output                      |
| `call.<res>.<method>`    | Call block result                  |
| `self`                   | Current resource (in provisioners) |
| `count.index`            | Current count iteration index      |
| `each.key`               | Current for_each key               |
| `each.value`             | Current for_each value             |
| `path.module`            | Path to the current module         |
| `path.root`              | Path to the root module            |
| `path.cwd`               | Current working directory          |
| `pulumi.stack`           | Current stack name                 |
| `pulumi.project`         | Current project name               |
| `pulumi.organization`    | Current organization name          |

### Operators

| Category   | Operators                        |
|------------|----------------------------------|
| Arithmetic | `+`, `-`, `*`, `/`, `%`          |
| Comparison | `==`, `!=`, `<`, `<=`, `>`, `>=` |
| Logical    | `&&`, `\|\|`, `!`                |

### Conditional Expression

```hcl
condition ? true_value : false_value
```

### For Expressions

```hcl
# List comprehension
[for name in var.names : upper(name)]

# With index
[for i, name in var.names : "${i}-${name}"]

# Map comprehension
{for k, v in var.tags : k => upper(v)}

# With filter
[for name in var.names : name if name != ""]
```

### Splat Expressions

```hcl
# Equivalent to [for r in aws_instance.web : r.id]
aws_instance.web[*].id
```

### Property Access

```hcl
resource.name.property
resource.name["key"]
resource.name[0]
```

### `try` and `can`

```hcl
try(var.optional.nested.value, "default")
can(var.optional.nested.value)  # returns true or false
```

## Built-in Functions

Pulumi HCL supports nearly all Terraform built-in functions. Functions are grouped by category below.

### Numeric

`abs`, `ceil`, `floor`, `log`, `max`, `min`, `pow`, `signum`, `parseint`

### String

`chomp`, `endswith`, `format`, `formatlist`, `indent`, `join`, `lower`, `regex`, `regexall`, `replace`, `split`, `startswith`, `strcontains`, `strrev`, `substr`, `title`, `trim`, `trimprefix`, `trimsuffix`, `trimspace`, `upper`

### Collection

`alltrue`, `anytrue`, `chunklist`, `coalesce`, `coalescelist`, `compact`, `concat`, `contains`, `distinct`, `element`, `entries`, `flatten`, `index`, `keys`, `length`, `list`, `lookup`, `map`, `matchkeys`, `merge`, `one`, `range`, `reverse`, `setintersection`, `setproduct`, `setsubtract`, `setunion`, `slice`, `sort`, `sum`, `transpose`, `values`, `zipmap`

### Encoding

`base64decode`, `base64encode`, `base64gzip`, `csvdecode`, `jsondecode`, `jsonencode`, `textdecodebase64`, `textencodebase64`, `urlencode`, `yamldecode`, `yamlencode`

### Filesystem

`abspath`, `basename`, `dirname`, `file`, `filebase64`, `fileexists`, `fileset`, `pathexpand`, `templatefile`

### Date and Time

`formatdate`, `timeadd`, `timecmp`, `timestamp`

### Hash and Crypto

`base64sha256`, `base64sha512`, `bcrypt`, `filebase64sha256`, `filebase64sha512`, `filemd5`, `filesha1`, `filesha256`, `filesha512`, `md5`, `rsadecrypt`, `sha1`, `sha256`, `sha512`, `uuid`, `uuidv5`

### IP Network

`cidrhost`, `cidrnetmask`, `cidrsubnet`, `cidrsubnets`

### Type Conversion

`can`, `nonsensitive`, `sensitive`, `tobool`, `tolist`, `tomap`, `tonumber`, `toset`, `tostring`, `try`, `type`

### Pulumi-Specific Functions

| Function                       | Description                                                  |
|--------------------------------|--------------------------------------------------------------|
| `fileAsset(path)`              | Create a Pulumi `FileAsset` from a local file path           |
| `stringAsset(text)`            | Create a Pulumi `StringAsset` from a string value            |
| `remoteAsset(uri)`             | Create a Pulumi `RemoteAsset` from a URL                     |
| `fileArchive(path)`            | Create a Pulumi `FileArchive` from a local path              |
| `remoteArchive(uri)`           | Create a Pulumi `RemoteArchive` from a URL                   |
| `assetArchive(map)`            | Create a Pulumi `AssetArchive` from a map of assets/archives |
| `pulumiResourceName(resource)` | Get the logical name from a resource's URN                   |
| `pulumiResourceType(resource)` | Get the type token from a resource's URN                     |

### Functions Not Supported

These Terraform functions have no equivalent:

| Function          | Reason                                        |
|-------------------|-----------------------------------------------|
| `templatestring`  | Inline template rendering not supported       |
| `plantimestamp`   | No Pulumi equivalent for plan-time timestamps |
| `ephemeralasnull` | Pulumi has no ephemeral value concept         |
| `issensitive`     | Not implemented                               |

## Stack References

Access outputs from other Pulumi stacks using the `pulumi_stackreference` resource:

```hcl
resource "pulumi_stackreference" "network" {
  name = "myorg/networking/prod"
}

output "vpc_id" {
  value = pulumi_stackreference.network.outputs["vpc_id"]
}
```

## Pulumi Block

The `pulumi` block configures Pulumi-specific settings.

### Version Constraints

```hcl
pulumi {
  required_version_range = ">= 3.0.0"
}
```

### Multi-Language Components

The `component` and `package` blocks declare an HCL module as a reusable component consumable from any Pulumi language. See [Multi-Language Components](mlc.md) for full details.

```hcl
pulumi {
  component {
    name   = "VpcNetwork"
    module = "index"
  }
  package {
    name    = "my-networking"
    version = "1.0.0"
  }
}
```

## Terraform Compatibility

Pulumi HCL is broadly compatible with Terraform's HCL syntax. This section covers the differences.

### The One Required Change

Provider sources must use the `pulumi/` namespace:

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

### Behavioral Differences

**Resource replacement order** — Pulumi creates the new resource before deleting the old one (opposite of Terraform). Set `create_before_destroy = false` in the `lifecycle` block to get Terraform's delete-first behavior.

**Sensitive values** — Variables and outputs marked `sensitive = true` become Pulumi secrets, encrypted at rest in state.

**Property names** — HCL uses `snake_case`. The plugin automatically converts to Pulumi's `camelCase` for the engine. Map keys are not translated.

### Feature Mappings

| Terraform Feature       | Pulumi Equivalent      | Notes                               |
|-------------------------|------------------------|-------------------------------------|
| `prevent_destroy`       | `protect`              | Same behavior                       |
| `ignore_changes`        | `ignoreChanges`        | Same behavior                       |
| `create_before_destroy` | `deleteBeforeReplace`  | Inverted logic                      |
| `moved` blocks          | `aliases`              | Renames without recreation          |
| `import` blocks         | Import resource option | Imports existing resources          |
| `timeouts`              | `customTimeouts`       | Same duration format                |
| Modules                 | Component resources    | All source types supported          |
| Provisioners            | Command provider       | `local-exec`, `remote-exec`, `file` |

### Ignored Blocks

These are parsed for compatibility but have no effect:

```hcl
terraform {
  backend "s3" { }        # Use pulumi login instead
  cloud { }               # Use Pulumi Cloud instead
  required_version = ""   # Use pulumi { required_version_range } instead
}
```

### Unsupported Features

- **`replace_triggered_by`** — Terraform cascades replacement when *other* resources change. Pulumi's `replaceOnChanges` triggers replacement when properties on *this* resource change. These have different semantics. Using `replace_triggered_by` produces an error.
- **WinRM connections** — Only SSH is supported in `connection` blocks.

### CLI Equivalents

| Terraform           | Pulumi           |
|---------------------|------------------|
| `terraform plan`    | `pulumi preview` |
| `terraform apply`   | `pulumi up`      |
| `terraform destroy` | `pulumi destroy` |
| `terraform state`   | `pulumi state`   |
| `terraform import`  | `pulumi import`  |
| Workspaces          | Stacks           |
