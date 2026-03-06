config "aList" "list(string)" {
}

config "singleOrNoneList" "list(string)" {
}

config "aString" "string" {
}

output "elementOutput1" {
  value = element(aList, 1)
}

output "elementOutput2" {
  value = element(aList, 2)
}

output "joinOutput" {
  value = join("|", aList)
}

output "lengthOutput" {
  value = length(aList)
}

output "splitOutput" {
  value = split("-", aString)
}

output "singleOrNoneOutput" {
  value = [singleOrNone(singleOrNoneList)]
}

