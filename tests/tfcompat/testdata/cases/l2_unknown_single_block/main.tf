resource "simple_resource" "dep" {
  input_one = "a"
}

resource "blocky_thing" "this" {
  name = "x"
  settings {
    mode = simple_resource.dep.result
  }
}

output "mode" {
  value = blocky_thing.this.settings[0].mode
}
