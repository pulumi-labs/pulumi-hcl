# A resource type whose schema has a `version` attribute (mirrors
# aws_rds_engine_version).
resource "simple_resource" "with_version" {
  input_one = "hello"
  input_two = false
  version   = "15.4"
}

output "result" { value = simple_resource.with_version.result }
