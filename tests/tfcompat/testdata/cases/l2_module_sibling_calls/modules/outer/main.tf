variable "name" {
  type = string
}

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
