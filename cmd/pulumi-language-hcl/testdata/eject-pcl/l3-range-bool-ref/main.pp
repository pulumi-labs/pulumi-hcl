resource "boolResource" "nestedobject:index:Target" {
  name = "bool-resource"
  options {
    range = createBool
  }
}

resource "boolTarget" "nestedobject:index:Target" {
  name = "${boolResource.name}+"
}

config "createBool" "bool" {
}

