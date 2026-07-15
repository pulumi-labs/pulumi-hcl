# `dependent` lists the WHOLE `middle` resource in replace_triggered_by, not
# one of its attributes. Per OpenTofu, a whole-instance reference triggers
# replacement whenever the referenced instance is planned to be updated or
# replaced -- the decision is based on the planned action, not on whether any
# attribute value changes.
#
# `middle` is itself replaced across the two stages (via replace_triggered_by
# on `driver.result`), but `middle`'s own inputs are constant, so all of
# `middle`'s attributes are identical before and after that replacement.

resource "replacer_resource" "driver" {
  force = "a"
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
