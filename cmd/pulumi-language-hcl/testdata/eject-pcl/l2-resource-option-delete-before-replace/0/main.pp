resource "withOption" "simple:index:Resource" {
  value = true
  options {
    replaceOnChanges    = [value]
    deleteBeforeReplace = true
  }
}

resource "withoutOption" "simple:index:Resource" {
  value = true
  options {
    replaceOnChanges = [value]
  }
}

