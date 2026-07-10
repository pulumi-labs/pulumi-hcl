# A `dynamic` block whose `for_each` is unknown at preview time
# (it reads a computed attribute of a not-yet-created resource).
# The whole `tag` property must render as unknown during preview
# and expand normally at apply.
resource "simple_resource" "upstream" {
  input_one = "a"
  input_two = true
}

resource "blocky_thing" "x" {
  name = "y"

  dynamic "tag" {
    for_each = simple_resource.upstream.result == "never" ? ["a"] : ["b"]
    content {
      key   = "k"
      value = tag.value
    }
  }
}

output "summary" { value = blocky_thing.x.summary }
