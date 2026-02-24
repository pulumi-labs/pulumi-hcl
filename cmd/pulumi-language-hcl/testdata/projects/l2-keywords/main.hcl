terraform {
  required_providers {
    keywords = {
      source  = "pulumi/keywords"
      version = "20.0.0"
    }
  }
}

resource "keywords_someresource" "firstResource" {
  builtins = "builtins"
  property = "property"
}
resource "keywords_someresource" "secondResource" {
  builtins = keywords_someresource.firstResource.builtins
  property = keywords_someresource.firstResource.property
}
