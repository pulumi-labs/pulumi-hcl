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

resource "snake_names_cool_module_some_resource" "first" {
  the_input = true
  nested = {
    nested_value = "nested"
  }
}
resource "snake_names_cool_module_another_resource" "third" {
  the_input = data.snake_names_cool_module_some_data.invoke_0.nested_output[0]["key"].value
}
