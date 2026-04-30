terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_getinfo" "info" {
}

resource "test_sink" "snakeSink" {
  value = data.test_getinfo.info[0].snake_case_field
}
resource "test_sink" "tagSink" {
  value = data.test_getinfo.info[0].tags_map.UserKey
}
