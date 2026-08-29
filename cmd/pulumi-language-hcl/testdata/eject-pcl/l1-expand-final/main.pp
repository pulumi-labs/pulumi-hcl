output "expandedMax" {
  value = notImplemented("max([1, 2, 3]...)")
}

output "expandedMaxWithPrefix" {
  value = notImplemented("max(0, [1, 2, 3]...)")
}

