resource "withOption" "simple:index:Resource" {
  value = false
  options {
    replaceOnChanges    = [value]
    deleteBeforeReplace = true
  }
}

resource "withoutOption" "simple:index:Resource" {
  value = false
  options {
    replaceOnChanges = [value]
  }
}

