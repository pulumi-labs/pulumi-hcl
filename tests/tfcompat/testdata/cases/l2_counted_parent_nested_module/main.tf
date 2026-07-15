module "outer" {
  source = "./outer"
  count  = 2
  base   = "b${count.index}"
}

output "all" {
  value = [for m in module.outer : m.combined]
}
