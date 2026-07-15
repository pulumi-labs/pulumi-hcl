variable "base" {
  type = string
}

module "inner" {
  source = "./inner"
  v      = var.base
}

output "combined" {
  value = module.inner.doubled
}
