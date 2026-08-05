# Stage 1: remove the whole `settings` block. Because `mode` (ForceNew) is
# only ever addressable as settings[0].mode, ignore_changes cannot reset it
# once the block is gone, so the ForceNew change is observed and the resource
# is REPLACED, with `settings` reporting the empty list the replacement was
# created with — matching OpenTofu. The terraform-provider plugin path also
# replaces, but its `settings` output reports the removed block's old value —
# the divergence the tfcompat case of the same name is skipped for.
resource "fnblock_resource" "r" {
  note = "y"
  lifecycle { ignore_changes = [settings[0].mode] }
}

output "settings" { value = fnblock_resource.r.settings }
