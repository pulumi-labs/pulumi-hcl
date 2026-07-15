resource "importable_resource" "web" {}

output "web_name" {
  value = importable_resource.web.name
}
