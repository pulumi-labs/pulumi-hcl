module "greeter" {
  source = "./modules/greeter"
  name   = "world"
}

output "result" {
  value = module.greeter.result
}

output "echo" {
  value = module.greeter.echo
}
