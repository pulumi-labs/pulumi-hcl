resource "pfx_res" "this" {
}

output "value" {
  value = pfx_res.this.attr[0].nested_attr[0].value
}
