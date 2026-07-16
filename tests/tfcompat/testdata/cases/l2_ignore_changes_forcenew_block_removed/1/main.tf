# Stage 1: remove the whole `settings` block. Because `mode` (ForceNew) is only
# ever addressable as settings[0].mode, ignore_changes cannot reset it once the
# block is gone, so OpenTofu sees the ForceNew attribute change and REPLACES the
# resource (destroy + create). pulumi-hcl instead updates it in place.
resource "fnblock_resource" "r" {
  note = "y"
  lifecycle { ignore_changes = [settings[0].mode] }
}

output "settings" { value = fnblock_resource.r.settings }
