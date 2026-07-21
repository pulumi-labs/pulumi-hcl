# A `depends_on` entry naming a module call covers every resource the module
# contains, so the read waits for `module.maker`'s pending creation just as it
# would for a resource written directly in the root.
module "maker" {
  source = "./modules/maker"
}

data "pending_lookup" "lookup" {
  name = "widget"

  depends_on = [module.maker]
}

output "looked_up" {
  value = data.pending_lookup.lookup.result
}
