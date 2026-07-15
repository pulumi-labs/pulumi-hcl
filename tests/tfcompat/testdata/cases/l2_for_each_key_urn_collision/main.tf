# Two distinct resource addresses whose Pulumi URN names collide.
#
# OpenTofu addresses these as `simple_resource.r["a-b"]` and
# `simple_resource.r-a["b"]` — distinct instances, applies cleanly.
#
# pulumi-hcl derives the URN name by joining the logical name and the
# for_each key with a "-" separator (buildResourceName), so:
#   r    + "-" + "a-b"  ->  "r-a-b"
#   r-a  + "-" + "b"    ->  "r-a-b"
# Both collapse onto the same URN and `pulumi up` fails with
# "Duplicate resource URN".
resource "simple_resource" "r" {
  for_each  = toset(["a-b"])
  input_one = each.key
  input_two = false
}

resource "simple_resource" "r-a" {
  for_each  = toset(["b"])
  input_one = each.key
  input_two = false
}

output "r_result" {
  value = simple_resource.r["a-b"].result
}

output "r_a_result" {
  value = simple_resource.r-a["b"].result
}
