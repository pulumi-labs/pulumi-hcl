data "pfx_obj_lookup" "this" {
  obj_attr = {
    item = ["a", "b"]
  }
}

output "value" {
  value = data.pfx_obj_lookup.this.value
}

output "item" {
  value = data.pfx_obj_lookup.this.obj_attr.item
}
