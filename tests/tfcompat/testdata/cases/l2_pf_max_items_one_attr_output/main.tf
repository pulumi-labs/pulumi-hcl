resource "pfx_flat" "this" {
  settings = [{ enabled = true }]
}

output "settings" {
  value = pfx_flat.this.settings
}

output "enabled" {
  value = pfx_flat.this.settings[0].enabled
}

output "count" {
  value = length(pfx_flat.this.settings)
}

resource "pfx_flat" "empty" {
  settings = []
}

output "empty_settings" {
  value = jsonencode(pfx_flat.empty.settings)
}
