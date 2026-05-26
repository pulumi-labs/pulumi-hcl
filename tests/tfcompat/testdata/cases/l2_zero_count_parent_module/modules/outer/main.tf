variable "subnets" { type = any }

locals {
  subnet_keys_with_tags = { for k, v in var.subnets : k => v.tags if can(v.tags) }
}

# Inner module's for_each depends on a local of THIS module. When the outer
# module has count=0, neither this inner module call nor its locals should
# run at all.
module "subnet_tags" {
  source   = "../inner"
  for_each = local.subnet_keys_with_tags
  tags     = each.value
}
