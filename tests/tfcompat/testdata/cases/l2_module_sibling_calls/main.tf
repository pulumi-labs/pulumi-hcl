module "outer" {
  source = "./modules/outer"
  name   = "hello"
}

output "results" {
  value = module.outer.results
}
