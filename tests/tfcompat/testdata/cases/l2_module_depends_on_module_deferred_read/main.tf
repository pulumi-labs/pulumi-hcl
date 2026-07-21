# The reader's `depends_on` names another module call rather than a resource:
# the dependency covers every resource `module.maker` contains, so the read
# inside `module.reader` waits for the maker's pending creation.
module "maker" {
  source = "./modules/maker"
}

module "reader" {
  source = "./modules/reader"

  depends_on = [module.maker]
}

output "looked_up" {
  value = module.reader.result
}
