# Two static blocks for a TypeList MaxItems=1 attribute. tofu rejects the
# config at plan time ("Too many settings blocks"); pulumi-hcl flattens
# MaxItemsOne blocks by keeping the last one, so the provider only ever sees a
# single block and the apply succeeds.
resource "blocky_thing" "x" {
  name = "y"

  settings {
    mode = "first"
  }

  settings {
    mode = "second"
  }
}
