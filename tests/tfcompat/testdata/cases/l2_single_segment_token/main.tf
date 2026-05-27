data "single" "ds" {
  query = "hi"
}

resource "single" "r" {
  input = "world"
}

output "data_answer" {
  value = data.single.ds.answer
}

output "resource_result" {
  value = single.r.result
}
