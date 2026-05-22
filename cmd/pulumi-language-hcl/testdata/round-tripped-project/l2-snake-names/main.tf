terraform {
  required_providers {
    snake_names = {
      source  = "pulumi/snake_names"
      version = "33.0.0"
    }
  }
}

data "snake_names_cool_module_some_data" "invoke_0" {
  the_input = snake_names_cool_module_some_resource.first.the_output["someKey"][0].nested_output
  nested {
    value = "fuzz"
  }
}

// Resource inputs are correctly translated
resource "snake_names_cool_module_some_resource" "first" {
  lifecycle {
    create_before_destroy = true
  }
  the_input = true
  nested = {
    nested_value = "nested"
  }
}
// Datasource outputs are correctly translated
resource "snake_names_cool_module_another_resource" "third" {
  lifecycle {
    create_before_destroy = true
  }
  the_input = data.snake_names_cool_module_some_data.invoke_0.nested_output[0]["key"].value
}
