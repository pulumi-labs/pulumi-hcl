resource "res" "large:index:String" {
  value = "hello world"
}

output "output" {
  value = res.value
}

