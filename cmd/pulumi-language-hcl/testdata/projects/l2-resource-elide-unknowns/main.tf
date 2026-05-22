terraform {
  required_providers {
    output = {
      source  = "pulumi/output"
      version = "23.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

// This test checks that when a provider doesn't return properties for fields it considers unknown the runtime
// can still access that field as an output.
provider "output" {
  alias          = "prov"
  elide_unknowns = true
}
resource "output_resource" "unknown" {
  provider = output.prov
  lifecycle {
    create_before_destroy = true
  }
  value = 1
}
resource "output_complex_resource" "complex" {
  provider = output.prov
  lifecycle {
    create_before_destroy = true
  }
  value = 1
}
// Try and use the unknown output as an input to another resource to check that it doesn't cause any issues.
resource "simple_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  value = output_resource.unknown.output == "hello"
}
resource "simple_resource" "resArray" {
  lifecycle {
    create_before_destroy = true
  }
  value = output_complex_resource.complex.output_array[0] == "hello"
}
resource "simple_resource" "resMap" {
  lifecycle {
    create_before_destroy = true
  }
  value = output_complex_resource.complex.output_map["x"] == "hello"
}
resource "simple_resource" "resObject" {
  lifecycle {
    create_before_destroy = true
  }
  value = output_complex_resource.complex.output_object.output == "hello"
}
// And try to use it has an output
output "out" {
  value = output_resource.unknown.output
}
output "outArray" {
  value = output_complex_resource.complex.output_array[0]
}
output "outMap" {
  value = output_complex_resource.complex.output_map["x"]
}
output "outObject" {
  value = output_complex_resource.complex.output_object.output
}
