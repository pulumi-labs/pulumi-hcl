# Collection values must serialize identically whether empty or populated. An
# empty set in particular must be an empty collection, not null; the populated
# cases and the other collection kinds guard against a regression that special-
# cases one kind or the empty case.

# Arrays (list and tuple).
output "empty_list"      { value = tolist([]) }
output "nonempty_list"   { value = tolist(["a", "b"]) }
output "empty_tuple"     { value = [] }
output "nonempty_tuple"  { value = ["a", 1, true] }

# Maps (map and object).
output "empty_map"       { value = tomap({}) }
output "nonempty_map"    { value = tomap({ k = "v" }) }
output "empty_object"    { value = {} }
output "nonempty_object" { value = { name = "n" } }

# Sets, including set operations that yield an empty set.
output "empty_set"         { value = toset([]) }
output "nonempty_set"      { value = toset(["a", "b"]) }
output "empty_setsubtract" { value = setsubtract(toset(["a"]), toset(["a"])) }
output "setproduct_empty"  { value = setproduct(toset(["a"]), toset([])) }
