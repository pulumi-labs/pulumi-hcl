module "items" {
  source = "./modules/item"
  count  = 2
  name   = "item-${count.index}"
}

output "first_name" {
  value = module.items[0].name
}

output "second_name" {
  value = module.items[1].name
}

output "all_names" {
  value = [for m in module.items : m.name]
}
