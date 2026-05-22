pulumi {
  required_providers {
    keywords = {
      source  = "pulumi/keywords"
      version = "20.0.0"
    }
  }
}

resource "keywords_some_resource" "firstResource" {
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
resource "keywords_some_resource" "secondResource" {
  builtins = keywords_some_resource.firstResource.builtins
  lambda   = keywords_some_resource.firstResource.lambda
  property = keywords_some_resource.firstResource.property
}
resource "keywords_lambda_some_resource" "lambdaModuleResource" {
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
resource "keywords_module_lambda" "lambdaResource" {
  builtins = "builtins"
  lambda   = "lambda"
  property = "property"
}
