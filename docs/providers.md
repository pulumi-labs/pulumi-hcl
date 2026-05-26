# Providers

Pulumi HCL discovers providers are discovered the same way as `opentofu`. If
`terraform.required_providers` is set, then that overrides the default provider
information.

Non-Pulumi providers are bridged into Pulumi's provider ecosystem using
[`pulumi-terraform-provider`](https://www.pulumi.com/registry/packages/terraform-provider/). After changing the set of providers used, you
must run `pulumi install`.

## Usage

### Default

By default, providers are resolved against the [OpenTofu registry](https://opentofu.org/registry/).

```hcl
# Resolves to "registry.opentofu.org/hashicorp/aws".
resource aws_s3_bucket "example" {}
```

This is equivalent to using `pulumi package add aws` in a Pulumi program from
another language.

### Specifying source & version

You can use `terraform.required_providers` to specify a full source & version
pair for a Terraform provider, exactly like `opentofu`:

```hcl
terraform {
  required_providers {
    <name> = {
      source  = "<source>"
      version = "<version constraint>"
    }
  }
}
```

```hcl
terraform {
  required_providers {
    mycloud = {
      source  = "mycorp/mycloud"
      version = "~> 1.0"
    }
  }
}
```

This is equivalent to using `pulumi package add mycorp/mycloud "~> 1.0"` in a
Pulumi program from another language.

### Specifying version

Pulumi HCL also supports the version only syntax in
`terraform.required_providers`:

```hcl
terraform {
  required_providers {
    mycloud = "~> 1.0"
  }
}
```

This is equivalent to the source & version syntax, with the `source` assumed to
be equivalent to the `name`.

### Pulumi Providers

You can consume pulumi providers directly by specifying a `source` prefixed with
`pulumi/`:

```hcl
terraform {
  required_providers {
    mycloud = {
      source  = "pulumi/pulumiservice"
      version = "1.1.0"
    }
  }
}
```

Pulumi providers require a semver version, instead of a full version constraint.
