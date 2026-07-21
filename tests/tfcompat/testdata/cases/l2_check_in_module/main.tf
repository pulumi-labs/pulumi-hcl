module "checked" {
  source = "./modules/checked"
  name   = "world"
}

output "result" {
  value = module.checked.result
}
