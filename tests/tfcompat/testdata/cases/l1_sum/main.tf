# Terraform's `sum` accumulates with arbitrary-precision numbers, so the
# intermediate additions stay exact rather than being rounded through float64.
# 9007199254740993 is not representable as a float64, but the exact sum
# 9007199254740994 is, so a precise accumulation round-trips through the output.
output "big_ints" {
  value = sum([9007199254740993, 1])
}

output "decimals" {
  value = sum([0.1, 0.2])
}

output "mixed" {
  value = sum([1, 2, 3, 4])
}
