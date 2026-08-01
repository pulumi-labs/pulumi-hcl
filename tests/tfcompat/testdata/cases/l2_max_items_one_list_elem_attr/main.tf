resource "blocky_thing" "this" {
  name = "a"
  pair = [["x", "y"]]
}

output "pair" {
  value = blocky_thing.this.pair
}

output "cell" {
  value = blocky_thing.this.pair[0][1]
}

output "row_len" {
  value = length(blocky_thing.this.pair[0])
}
