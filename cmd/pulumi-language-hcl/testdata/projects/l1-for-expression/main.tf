locals {
  names = ["alpha", "beta", "gamma"]
}
locals {
  tags = {
    "Environment" = "production"
    "Team"        = "infra"
  }
}
output "prefixed" {
  value = [for n in local.names :"prefix-${n}"]
}
output "filtered" {
  value = [for n in local.names : n if n != "beta"]
}
output "indexed" {
  value = [for i, n in local.names :"${i}:${n}"]
}
output "tagList" {
  value = [for k, v in local.tags :"${k}=${v}"]
}
output "prefixedMap" {
  value = {for n in local.names : n =>"prefix-${n}"}
}
output "filteredTags" {
  value = {for k, v in local.tags : k => v if k != "Team"}
}
