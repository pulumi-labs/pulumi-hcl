# A template rendered by `templatefile` may itself call the other template
# functions, exactly as OpenTofu does: OpenTofu exposes `templatestring` (and a
# recursion-limited `templatefile`) inside the rendered template. Here the outer
# template calls `templatestring` to render an inner template.
output "nested" {
  value = templatefile("${path.module}/outer.tftpl", {})
}
