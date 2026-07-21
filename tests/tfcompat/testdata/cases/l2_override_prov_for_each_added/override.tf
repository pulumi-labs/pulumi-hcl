provider "simple" {
  alias    = "alt"
  for_each = toset(["x"])
}
