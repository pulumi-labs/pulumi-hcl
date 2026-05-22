terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "import" {
  lifecycle {
    create_before_destroy = true
  }
  import_id = "fakeID123"
  value     = true
}
resource "simple_resource" "notImport" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
