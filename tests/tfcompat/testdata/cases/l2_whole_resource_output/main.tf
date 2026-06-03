# Referencing a resource as a whole object (rather than a single attribute)
# must expose only the resource's real attributes. pulumi-hcl attaches a
# synthetic `urn` attribute to every resource output object internally (so
# `pulumiResourceName`, `depends_on`, etc. resolve); OpenTofu has no such
# attribute, so a whole-resource output must not leak it.
#
# simple_resource has a deterministic id ("simple-id") and computed `result`
# derived from inputs, so the whole object is fully comparable across runtimes.
resource "simple_resource" "single" {
  input_one = "hello"
  input_two = true
}

resource "simple_resource" "counted" {
  count     = 2
  input_one = "c${count.index}"
  input_two = false
}

resource "simple_resource" "each" {
  for_each  = toset(["x", "y"])
  input_one = each.key
  input_two = false
}

# Whole single resource.
output "single" {
  value = simple_resource.single
}

# Whole count resource: a tuple of objects.
output "counted" {
  value = simple_resource.counted
}

# Whole for_each resource: an object keyed by each.key.
output "each" {
  value = simple_resource.each
}
