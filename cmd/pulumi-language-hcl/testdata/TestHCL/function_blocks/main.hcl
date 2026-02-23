terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_getfiltered" "invoke_0" {
  name = "my-filter"
  filters {
    key   = "tag:Name"
    value = "production"
  }
  filters {
    key   = "tag:Env"
    value = "prod"
  }
}

output "filteredId" {
  value = data.test_getfiltered.invoke_0.id
}
