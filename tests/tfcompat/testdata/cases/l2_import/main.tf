resource "importable_resource" "foo" {
  for_each = toset(["a", "b"])
}

import {
  to = importable_resource.foo["a"]
  id = "id-a"
}

import {
  to = importable_resource.foo["b"]
  id = "id-b"
}

output "names" {
  value = { for k, r in importable_resource.foo : k => r.name }
}

resource "importable_resource" "bar" {
  count = 2
}

import {
  to = importable_resource.bar[1]
  id = "id-1"
}

output "bar_names" {
  value = importable_resource.bar[*].name
}

# The root import block is listed before the module-addressed one: if import
# matching ignored the module path, first-match-wins would hand "id-root" to
# the module's same-named resource.
resource "importable_resource" "web" {}

import {
  to = importable_resource.web
  id = "id-root"
}

import {
  to = module.child.importable_resource.web
  id = "id-child"
}

module "child" {
  source = "./modules/child"
}

output "web_name" {
  value = importable_resource.web.name
}

output "child_web_name" {
  value = module.child.web_name
}
