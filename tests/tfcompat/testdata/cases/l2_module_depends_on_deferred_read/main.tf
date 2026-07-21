# `depends_on` on a module call applies to everything the module contains,
# data sources included: OpenTofu holds the child's read until after
# `pending_thing.thing` is created, even though the read's own arguments are
# known and the module takes no input from the resource.
resource "pending_thing" "thing" {
  name = "widget"
}

module "reader" {
  source = "./modules/reader"

  depends_on = [pending_thing.thing]
}

output "looked_up" {
  value = module.reader.result
}
