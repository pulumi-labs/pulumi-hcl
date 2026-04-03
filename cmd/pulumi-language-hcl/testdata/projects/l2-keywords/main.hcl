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
  lambda   = "lambda"
  property = "property"
}
resource "keywords_someresource" "secondResource" {
  builtins = keywords_someresource.firstResource.builtins
  lambda   = keywords_someresource.firstResource.lambda
  property = keywords_someresource.firstResource.property
}
resource "keywords_lambda_someresource" "lambdaModuleResource" {
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
resource "keywords_lambda" "lambdaResource" {
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
