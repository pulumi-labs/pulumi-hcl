variable "marker_dir" {
  type = string
}

resource "terraform_data" "guarded" {
  input = var.marker_dir

  provisioner "local-exec" {
    command = "touch '${var.marker_dir}/created'"
  }

  provisioner "local-exec" {
    when    = destroy
    command = "touch '${self.input}/destroyed'"
  }
}
