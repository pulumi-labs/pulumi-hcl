# OpenTofu's `lookup(map, key, default)` returns the map's element type: when
# the key is absent the default is converted to that element type before it is
# returned. A numeric default into a map(string) therefore comes back as a
# string, and a string default into a map(number) comes back as a number.
#
# The results are wrapped in an object so the harness serializes them as JSON,
# which preserves the scalar type (a bare top-level scalar would stringify and
# hide the string-vs-number difference).
output "results" {
  value = {
    num_default_into_string_map    = lookup(tomap({ a = "1" }), "missing", 30)
    string_default_into_number_map = lookup(tomap({ a = 1 }), "missing", "80")
    bool_default_into_string_map   = lookup(tomap({ a = "x" }), "missing", true)
  }
}
