resource "mark_resource" "example" {}

check "ordering_check" {
  data "mark_probe" "probe" {
    token = mark_resource.example.token
  }

  assert {
    condition     = data.mark_probe.probe.constructed
    error_message = "scoped data source ran before mark_resource was constructed"
  }
}

output "token" {
  value = mark_resource.example.token
}
