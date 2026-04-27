resource "class" "simple:index:Resource" {
  value = true
}

resource "export" "simple:index:Resource" {
  value = true
}

resource "mod" "simple:index:Resource" {
  value = true
}

resource "import" "simple:index:Resource" {
  value = true
}

# TODO(pulumi/pulumi#18246): Pcl should support scoping based on resource type just like HCL does in TF so we can uncomment this.
# output "import" {
#   value = Resource["import"]
# }
resource "object" "simple:index:Resource" {
  value = true
}

resource "self" "simple:index:Resource" {
  value = true
}

resource "this" "simple:index:Resource" {
  value = true
}

resource "if" "simple:index:Resource" {
  value = true
}

output "class" {
  value = class
}

output "export" {
  value = export
}

output "mod" {
  value = mod
}

output "object" {
  value = object
}

output "self" {
  value = self
}

output "this" {
  value = this
}

output "if" {
  value = if
}

