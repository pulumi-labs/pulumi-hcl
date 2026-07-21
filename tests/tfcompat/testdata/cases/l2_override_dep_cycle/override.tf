resource "simple_resource" "a" {
  input_one = "no-longer-depends-on-b"
}
