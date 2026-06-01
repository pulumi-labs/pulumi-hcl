output "rendered" {
  value = templatefile("${path.module}/greeting.tmpl", { name = "ada", nums = [1, 2, 3] })
}
