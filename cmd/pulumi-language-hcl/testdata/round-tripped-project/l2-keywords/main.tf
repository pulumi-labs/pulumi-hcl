terraform {
  required_providers {
    keywords = {
      source  = "pulumi/keywords"
      version = "20.0.0"
    }
  }
}

resource "keywords_someresource" "firstResource" {
  lifecycle {
    create_before_destroy = true
  }
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
resource "keywords_someresource" "secondResource" {
  lifecycle {
    create_before_destroy = true
  }
  builtins = keywords_someresource.firstResource.builtins
  lambda   = keywords_someresource.firstResource.lambda
  property = keywords_someresource.firstResource.property
}
resource "keywords_lambda_someresource" "lambdaModuleResource" {
  lifecycle {
    create_before_destroy = true
  }
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
resource "keywords_module_lambda" "lambdaResource" {
  lifecycle {
    create_before_destroy = true
  }
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
