provider "simple" {
  prefix = "from-root"
}

module "mid" {
  source = "./modules/mid"
}

output "module_prefix_result" {
  value = module.mid.prefix_result
}
