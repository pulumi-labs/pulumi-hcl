data "pfx_lookup" "d" {}

# pfx_lookup (a plugin-framework data source) declares only `value` and no `id`.
# Referencing the whole object must yield the same attribute set on both runtimes.
output "keys" {
  value = sort(keys(data.pfx_lookup.d))
}
