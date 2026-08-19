resource "typeclash_res" "r" {
  block {
    name = "a"
    nested_block {
      inner_block {
        attr = "x"
      }
    }
  }
}

resource "typeclash_res_block" "b" {
  nested_block {
    inner_block {
      attr = "y"
    }
    inner_block {
      attr = "z"
    }
  }
}

output "res" {
  value = typeclash_res.r.summary
}

output "res_block" {
  value = typeclash_res_block.b.summary
}
