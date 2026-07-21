module "thing" {
  source = "./modules/thing"
}

output "input_one" { value = module.thing.input_one }
output "input_two" { value = module.thing.input_two }
