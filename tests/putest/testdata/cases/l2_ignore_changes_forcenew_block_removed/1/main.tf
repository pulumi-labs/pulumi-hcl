# Stage 1: remove the whole `settings` block. Like OpenTofu, pulumi-hcl
# observes the ForceNew `mode` change (the ignored index no longer exists) and
# replaces the resource, sending the replacement `settings = []`. Unlike
# OpenTofu, the `settings` output still reports the removed block's old value
# instead of the empty list the replacement was created with — see the skipped
# tfcompat case of the same name.
resource "fnblock_resource" "r" {
  note = "y"
  lifecycle { ignore_changes = [settings[0].mode] }
}

output "settings" { value = fnblock_resource.r.settings }
