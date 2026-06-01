terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  for_each = toset(flatten([for a in [{
    "items" = [{
      "items" = [{
        "items" = ["alpha", "bravo"]
      }]
    }]
    }, {
    "items" = [{
      "items" = [{
        "items" = ["charlie"]
      }]
    }]
  }] : [for b in try(a.items, []) : [for c in try(b.items, []) : try(c.items, [])]]]))
  input = each.value
}

output "results" {
  value = [for a in [{
    "items" = [{
      "items" = [{
        "items" = ["alpha", "bravo"]
      }]
    }]
    }, {
    "items" = [{
      "items" = [{
        "items" = ["charlie"]
      }]
    }]
  }] : [for b in try(a.items, []) : [for c in try(b.items, []) : [for d in try(c.items, []) : data.test_echo.invoke_0[d].result]]]]
}
