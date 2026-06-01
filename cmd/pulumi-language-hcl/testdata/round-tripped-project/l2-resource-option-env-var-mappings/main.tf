terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

provider "simple" {
  alias = "prov"
  pulumi {
    env_var_mappings = {
      "MY_VAR"    = "PROVIDER_VAR"
      "OTHER_VAR" = "TARGET_VAR"
    }
  }
}
