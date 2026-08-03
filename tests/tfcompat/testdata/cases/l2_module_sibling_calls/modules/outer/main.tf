variable "name" {
  type = string
}

# Two calls in one module, sharing a source: the components are distinct
# instances ("outer.a", "outer.b") of the same component type.
module "a" {
  source = "./modules/leaf"
  name   = "${var.name}-a"
}

module "b" {
  source = "./modules/leaf"
  name   = "${var.name}-b"
}

output "results" {
  value = [module.a.result, module.b.result]
}
