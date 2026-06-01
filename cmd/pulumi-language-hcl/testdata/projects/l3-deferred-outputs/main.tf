terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

# first & second are simple mutually dependent components
module "first" {
  source = "./first"
  input  = module.second.untainted
}
module "second" {
  source = "./second"
  input  = module.first.untainted
}
# another & many are also mutually dependent components, but many tests that the mutual dependency works through
# `range`.
module "another" {
  source = "./first"
  # We do the join + for + == dance because we want to force a value that depends on the contents of the list, not
  # just it's length (which may be known at preview time).
  input = join("", [for _, v in module.many : v.untainted ? "a" : "b"]) == "xyz"
}
module "many" {
  source = "./second"
  count  = 2
  input  = module.another.untainted
}
