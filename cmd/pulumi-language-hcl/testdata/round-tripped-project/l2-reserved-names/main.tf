terraform {
  required_providers {
    reservednames = {
      source  = "pulumi/reservednames"
      version = "51.0.0"
    }
  }
}

# A resource whose `elementType` property collides with the `ElementType()` method that
# generated Go SDK types must implement.
resource "reservednames_element_type" "elem" {
  lifecycle {
    create_before_destroy = true
  }
  element_type = {
    element_type = "nested"
  }
}
output "elementType" {
  value = reservednames_element_type.elem.element_type.element_type
}
