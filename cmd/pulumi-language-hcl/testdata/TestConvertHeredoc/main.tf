terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_res" "normal" {
  lifecycle {
    create_before_destroy = true
  }
  prop = <<EON
 hello
world
EON

}
resource "test_res" "indented" {
  lifecycle {
    create_before_destroy = true
  }
  prop = <<-EOI
hello: ${test_res.normal.id}
 world
EOI

}
