module "outer" {
  source = "./modules/outer"
  name   = "hello"
}

output "outer_result" {
  value = module.outer.result
}

output "outer_inner_passthrough" {
  value = module.outer.inner_passthrough
}

output "root_paths" {
  value = {
    module = path.module
    root   = path.root
    cwd    = path.cwd
  }
}

output "outer_paths" {
  value = module.outer.paths
}

output "inner_paths" {
  value = module.outer.inner_paths
}
