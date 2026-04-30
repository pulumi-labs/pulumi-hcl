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
  value = data.test_getinfo.info.snake_case_field
}
resource "test_sink" "tagSink" {
  value = data.test_getinfo.info.tags_map.UserKey
}
