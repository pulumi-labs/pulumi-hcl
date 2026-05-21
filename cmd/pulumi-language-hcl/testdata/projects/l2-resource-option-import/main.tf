terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "import" {
  import_id = "fakeID123"
  value     = true
}
resource "simple_resource" "notImport" {
  value = true
}
