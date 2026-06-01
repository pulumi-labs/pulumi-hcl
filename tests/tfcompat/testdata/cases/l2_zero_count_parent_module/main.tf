# Outer module has count = 0 (analogous to aws-ia/rds-aurora's
# `count = var.setup_globaldb ? 1 : 0` on the secondary VPC). The outer
# module has its own inner `module` blocks; those must NOT be instantiated
# when count=0 — tofu skips them entirely. pulumi-hcl was incorrectly
# processing them against the root eval ctx, which failed to find the
# outer module's locals.
module "secondary_vpc" {
  source = "./modules/outer"
  count  = 0
  subnets = {
    public  = { netmask = 20 }
    private = { netmask = 20 }
  }
}

# Anchor so both runtimes have at least one resource registration.
output "anchor" { value = "anchor" }
