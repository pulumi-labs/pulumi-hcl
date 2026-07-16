resource "pfx_obj" "this" {
  obj_attr = {
    item = ["a", "b"]
  }
}

output "value" {
  value = pfx_obj.this.obj_attr.item
}
