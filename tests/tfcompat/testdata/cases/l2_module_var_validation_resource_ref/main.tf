module "child" {
  source = "./child"
}

output "gate" {
  value = module.child.gate
}

output "result" {
  value = module.child.result
}
