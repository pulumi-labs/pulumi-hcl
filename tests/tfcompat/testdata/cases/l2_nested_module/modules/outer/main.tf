variable "name" {
  type = string
}

module "inner" {
  source = "./modules/inner"
  name   = "${var.name}-inner"
}

output "result" {
  value = module.inner.result
}

output "inner_passthrough" {
  value = module.inner.echoed_name
}

output "paths" {
  value = {
    module = path.module
    root   = path.root
    cwd    = path.cwd
  }
}

output "inner_paths" {
  value = module.inner.paths
}
