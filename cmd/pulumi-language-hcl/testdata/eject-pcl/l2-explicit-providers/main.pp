resource "explicit" "pulumi:providers:component" {
}

resource "list" "component:index:ComponentCallable" {
  value = "value"
  options {
    providers = [explicit]
  }
}

resource "map" "component:index:ComponentCallable" {
  value = "value"
  options {
    providers = [explicit]
  }
}

