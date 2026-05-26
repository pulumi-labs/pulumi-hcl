# Inner module defines its own aliased provider block; a resource inside
# the inner module references it via `provider = simple.alpha`. Both runtimes
# must resolve that reference against the inner module's eval context, not
# the root. Mirrors aws-ia/rds-aurora where the aurora module declares
# `provider "aws" { alias = "primary" }` then uses `provider = aws.primary`.
module "inner" {
  source = "./modules/inner"
  marker = "hi"
}

output "marker" { value = module.inner.marker }
