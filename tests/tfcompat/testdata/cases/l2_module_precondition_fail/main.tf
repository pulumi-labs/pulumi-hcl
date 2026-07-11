module "child" {
  source   = "./child"
  expected = "not-ok"
}

output "child_id" {
  value = module.child.id
}
