provider "simple" {
  alias  = "from_parent"
  prefix = "parent"
}

# A dotted key (`simple.foo`) on the left of a `providers = { ... }` entry
# is valid TF: it names the local aliased provider in the child's scope
# that the right-hand value should map onto. Parses fine in tofu;
# pulumi-hcl's parser rejects it as ambiguous.
module "child" {
  source = "./modules/child"
  providers = {
    simple.foo = simple.from_parent
  }
}

output "resource_prefix_result" {
  value = module.child.resource_prefix_result
}
