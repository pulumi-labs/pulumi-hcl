module "with_default" {
  source = "./modules/cfg"
  config = {
    name = "uses-default"
    # tag omitted — module's optional(string, "default-tag") must fill in
  }
}

module "with_override" {
  source = "./modules/cfg"
  config = {
    name = "uses-override"
    tag  = "overridden"
  }
}

output "default_tag" {
  value = module.with_default.tag
}

output "override_tag" {
  value = module.with_override.tag
}
