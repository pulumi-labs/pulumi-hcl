# OpenTofu's `length` propagates the sensitivity (and any other marks) of its
# argument onto the returned count. A sensitive tuple or object therefore yields
# a sensitive length. `issensitive` exposes that mark as a plain bool so the
# behavior is observable in outputs without leaking the sensitive value itself.
output "len_tuple_sensitive" {
  value = issensitive(length(sensitive(["a", "b", "c"])))
}

output "len_object_sensitive" {
  value = issensitive(length(sensitive({ a = 1, b = 2 })))
}

# A sensitive string, list, and map must stay sensitive too — these already
# propagate the mark and guard against a regression in the other branches.
output "len_string_sensitive" {
  value = issensitive(length(sensitive("hello")))
}

output "len_list_sensitive" {
  value = issensitive(length(sensitive(tolist(["a", "b"]))))
}

output "len_map_sensitive" {
  value = issensitive(length(sensitive(tomap({ a = "1" }))))
}
