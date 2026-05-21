pulumi {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_res" "normal" {
  prop = <<EON
 hello
world
EON

}
resource "test_res" "indented" {
  prop = <<-EOI
hello: ${test_res.normal.id}
 world
EOI

}
