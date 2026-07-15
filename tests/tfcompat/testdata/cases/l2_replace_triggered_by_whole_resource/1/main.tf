# Stage 1: `driver.force` changes a -> b. Because `force` is ForceNew, `driver`
# is replaced, so `driver.result` changes, which replaces `middle` via its
# replace_triggered_by. `middle`'s own inputs are unchanged, so after the
# replacement every attribute of `middle` is identical to before.
#
# OpenTofu: `middle` is planned to be replaced, so the whole-instance reference
# in `dependent.replace_triggered_by` forces `dependent` to be replaced too.
# pulumi-hcl: the reference is compared by value; `middle`'s value is
# unchanged, so `dependent` is NOT replaced.

resource "replacer_resource" "driver" {
  force = "b"
}

resource "replacer_resource" "middle" {
  force = "const"
  lifecycle {
    replace_triggered_by = [replacer_resource.driver.result]
  }
}

resource "replacer_resource" "dependent" {
  force = "dep"
  note  = "x"
  lifecycle {
    replace_triggered_by = [replacer_resource.middle]
  }
}

output "driver_result" {
  value = replacer_resource.driver.result
}

output "middle_result" {
  value = replacer_resource.middle.result
}

output "dependent_result" {
  value = replacer_resource.dependent.result
}
