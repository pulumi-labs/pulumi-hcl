module "child" {
  source   = "./child"
  expected = "ok"
}

output "child_id" {
  value = module.child.id
}
