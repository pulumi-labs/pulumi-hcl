terraform {
  required_providers {
    keywords = {
      source  = "pulumi/keywords"
      version = "20.0.0"
    }
  }
}

resource "keywords_some_resource" "firstResource" {
  lifecycle {
    create_before_destroy = true
  }
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
resource "keywords_some_resource" "secondResource" {
  lifecycle {
    create_before_destroy = true
  }
  builtins = keywords_some_resource.firstResource.builtins
  lambda   = keywords_some_resource.firstResource.lambda
  property = keywords_some_resource.firstResource.property
}
resource "keywords_lambda_some_resource" "lambdaModuleResource" {
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
