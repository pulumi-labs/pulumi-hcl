provider "simple" {
  prefix = "hello"
}

resource "simple_resource" "top" {
  input_one = "world"
  input_two = true
}

module "broken_simple_unused" {
  source = "./modules/broken_simple_unused"
}

output "top" {
  value = simple_resource.top.prefix_result
}

output "module_thing" {
  value = module.broken_simple_unused.thing_summary
}
