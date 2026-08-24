terraform {
  required_providers {
    nestedcollections = {
      source  = "pulumi/nestedcollections"
      version = "50.0.0"
    }
  }
}

# A resource with deeply nested collection output properties: a list of lists of lists
# of an object type and a map of maps of maps of strings.
resource "nestedcollections_foo" "foo" {
  lifecycle {
    create_before_destroy = true
  }
}
output "secondProp" {
  value = nestedcollections_foo.foo.condition_sets[0][0][1].prop
}
output "leaf" {
  value = nestedcollections_foo.foo.private_endpoint["outer"]["inner"]["leaf"]
}
