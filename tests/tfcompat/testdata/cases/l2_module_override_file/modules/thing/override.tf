# An `override.tf` file is merged into the module's other .tf files rather
# than being a separate declaration: attributes set here replace the ones in
# main.tf, and attributes left out keep their original value.
resource "simple_resource" "r" {
  input_one = "overridden"
}
