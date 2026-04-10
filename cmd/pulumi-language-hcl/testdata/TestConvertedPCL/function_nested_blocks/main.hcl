terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_blockinvoke" "invoke_0" {
  outer {
    inner {
      prop = true
    }
    inner {
      prop = false
    }
  }
  outer {
    inner {
      prop = false
    }
    inner {
      prop = true
    }
  }
}
data "test_blockinvoke" "invoke_1" {
}
data "test_blockinvoke" "invoke_2" {
  outer {
  }
}

output "result" {
  value = data.test_blockinvoke.invoke_0.id
}
output "emptyOuter" {
  value = data.test_blockinvoke.invoke_1.id
}
output "emptyInner" {
  value = data.test_blockinvoke.invoke_2.id
}
