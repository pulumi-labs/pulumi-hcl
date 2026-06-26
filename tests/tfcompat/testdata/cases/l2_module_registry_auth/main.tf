module "m" {
  source  = "registry.tfcompat.test/acme/widget/aws"
  version = "1.0.0"
}

output "greeting" {
  value = module.m.greeting
}
