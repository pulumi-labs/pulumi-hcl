pulumi {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_res" "normal" {
  prop = " hello\nworld\n"
}
resource "test_res" "indented" {
  prop = "hello\n world\n"
}
