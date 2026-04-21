terraform {
  required_providers {
    std = {
      source  = "pulumi/std"
      version = "1.0.0"
    }
  }
}

output "results" {
  value = [for _, v in {
    "a" = "ALPHA"
    "b" = "BRAVO"
  } : lower(v)]
}
