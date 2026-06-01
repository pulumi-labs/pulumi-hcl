module "child" {
  source = "./child"
  items  = null
  name   = ""
}

output "item_count" {
  value = module.child.count
}

output "name" {
  value = module.child.name
}
