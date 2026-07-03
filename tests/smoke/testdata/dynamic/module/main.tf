# A module exercising both provider-resolution paths of the fully dynamic MLC:
#   - random (source pulumi/random) resolves to a native Pulumi package, and
#   - null   (source hashicorp/null) resolves to a terraform-provider
#     parameterization through the handshake resolver.
terraform {
  required_providers {
    random = {
      source = "pulumi/random"
    }
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.5"
    }
  }
}

# A file next to the module, read through a bridged provider data source
# (https://github.com/pulumi-labs/pulumi-hcl/issues/305).
data "local_file" "version" {
  filename = "${path.module}/VERSION"
}

variable "prefix" {
  type = string
}

resource "random_string" "suffix" {
  length  = 8
  special = false
  upper   = false
}

# A bridged Terraform resource that depends on the native one, so both providers
# must resolve for the module to apply.
resource "null_resource" "marker" {
  triggers = {
    name = "${var.prefix}-${random_string.suffix.result}"
  }
}

output "name" {
  value = null_resource.marker.triggers.name
}

output "module_version" {
  value = trimspace(data.local_file.version.content)
}
