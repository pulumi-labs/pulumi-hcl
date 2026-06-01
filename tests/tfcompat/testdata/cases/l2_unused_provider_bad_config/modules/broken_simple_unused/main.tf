# Submodule declares its own `provider "simple"` with config that would
# fail ConfigureContextFunc. The only resource here uses the `blocky`
# provider, so `simple` is never instantiated in this module and tofu
# skips configuring it.
provider "simple" {
  fail_validate = "boom"
}

resource "blocky_thing" "t" {
  name = "in-module"
  settings {
    mode    = "fast"
    verbose = true
  }
}

output "thing_summary" {
  value = blocky_thing.t.summary
}
