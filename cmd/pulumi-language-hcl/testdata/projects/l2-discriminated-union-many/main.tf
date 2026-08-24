terraform {
  required_providers {
    discriminated-union-many = {
      source  = "pulumi/discriminated-union-many"
      version = "49.0.0"
    }
  }
}

resource "discriminated-union-many_example" "example1" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant1"
    payload           = "p1"
    extra             = "e1"
  }
}
resource "discriminated-union-many_example" "example2" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant2"
    payload           = "p2"
    extra             = "e2"
  }
}
resource "discriminated-union-many_example" "example3" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant3"
    payload           = "p3"
    count             = 3
  }
}
resource "discriminated-union-many_example" "example4" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant4"
    payload           = "p4"
    enabled           = true
  }
}
resource "discriminated-union-many_example" "example5" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant5"
    payload           = "p5"
    label             = "l5"
  }
}
resource "discriminated-union-many_example" "example6" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant6"
    payload           = "p6"
    code              = 6
  }
}
resource "discriminated-union-many_example" "example7" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant7"
    payload           = "p7"
    message           = "m7"
  }
}
resource "discriminated-union-many_example" "example8" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant8"
    payload           = "p8"
    size              = 8
  }
}
resource "discriminated-union-many_example" "example9" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant9"
    payload           = "p9"
    flag              = false
  }
}
resource "discriminated-union-many_example" "example10" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant10"
    payload           = "p10"
    note              = "n10"
  }
}
// A SubsetExample's unionOf is typed as a 3-variant subset union. We should be
// able to assign that output to an Example's unionOf, which is typed as the
// full 10-variant union.
resource "discriminated-union-many_subset_example" "subset1" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = {
    discriminant_kind = "variant3"
    payload           = "sp"
    count             = 33
  }
}
resource "discriminated-union-many_example" "example11" {
  lifecycle {
    create_before_destroy = true
  }
  union_of = discriminated-union-many_subset_example.subset1.union_of
}
