variable "aList" {
  type = list(string)
}
variable "singleOrNoneList" {
  type = list(string)
}
variable "aString" {
  type = string
}
output "elementOutput1" {
  value = element(var.aList, 1)
}
output "elementOutput2" {
  value = element(var.aList, 2)
}
output "joinOutput" {
  value = join("|", var.aList)
}
output "lengthOutput" {
  value = length(var.aList)
}
output "splitOutput" {
  value = split("-", var.aString)
}
output "singleOrNoneOutput" {
  value = [one(var.singleOrNoneList)]
}
