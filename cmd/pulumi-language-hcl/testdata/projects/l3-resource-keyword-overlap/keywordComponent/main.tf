terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

# A resource named `this` collides with the receiver pointer of the
# ComponentResource class generated for this component. NodeJS must rename the
# resource variable (e.g. to `_this`) while keeping the `parent: this` pointer
# intact.
resource "simple_resource" "this" {
  pulumi {
    name ="${pulumi.module.name}-this"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = var.input
}
# Referencing `this` exercises that the rename is applied to references too, not
# just the declaration. The name `parent` also overlaps with the `parent`
# resource-option key, which must not be confused with this resource variable.
resource "simple_resource" "parent" {
  pulumi {
    name ="${pulumi.module.name}-parent"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = simple_resource.this.value
}
variable "input" {
  type        = bool
  description = "An input passed to the component"
}
output "result" {
  value = simple_resource.parent.value
}
