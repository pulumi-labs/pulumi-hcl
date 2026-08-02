resource "composite_attachment" "a" {
  role   = "app-role"
  policy = "arn:policy/admin"
}

output "attached_role" {
  value = composite_attachment.a.role
}

output "attached_policy" {
  value = composite_attachment.a.policy
}
