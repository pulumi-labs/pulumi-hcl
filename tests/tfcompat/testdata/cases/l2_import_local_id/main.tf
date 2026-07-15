locals {
  the_id = "id-from-local"
}

import {
  to = importable_resource.target
  id = local.the_id
}

resource "importable_resource" "target" {}

output "target_name" {
  value = importable_resource.target.name
}
