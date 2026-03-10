variable "names" {
  type = list(string)
}
variable "tags" {
  type = map(string)
}
output "greetings" {
  value = [for _, name in var.names :"Hello, ${name}!"]
}
output "numbered" {
  value = [for i, name in var.names :"${i}-${name}"]
}
output "tagList" {
  value = [for k, v in var.tags :"${k}=${v}"]
}
output "greetingMap" {
  value = {for _, name in var.names : name =>"Hello, ${name}!"}
}
output "filteredList" {
  value = [for _, name in var.names : name if name != "b"]
}
output "filteredMap" {
  value = {for _, name in var.names : name =>"Hello, ${name}!"if name != "b"}
}
