terraform {
  required_providers {
    asset-archive = {
      source  = "pulumi/asset-archive"
      version = "5.0.0"
    }
  }
}

resource "asset-archive_asset_resource" "ass" {
  lifecycle {
    create_before_destroy = true
  }
  value = fileasset("../test.txt")
}
resource "asset-archive_archive_resource" "arc" {
  lifecycle {
    create_before_destroy = true
  }
  value = filearchive("../archive.tar")
}
resource "asset-archive_archive_resource" "dir" {
  lifecycle {
    create_before_destroy = true
  }
  value = filearchive("../folder")
}
resource "asset-archive_archive_resource" "assarc" {
  lifecycle {
    create_before_destroy = true
  }
  value = assetarchive({
    "string"  = stringasset("file contents")
    "file"    = fileasset("../test.txt")
    "folder"  = filearchive("../folder")
    "archive" = filearchive("../archive.tar")
  })
}
resource "asset-archive_asset_resource" "remoteass" {
  lifecycle {
    create_before_destroy = true
  }
  value = remoteasset("https://raw.githubusercontent.com/pulumi/pulumi/7b0eb7fb10694da2f31c0d15edf671df843e0d4c/cmd/pulumi-test-language/tests/testdata/l2-resource-asset-archive/test.txt")
}
resource "asset-archive_archive_resource" "remotearc" {
  lifecycle {
    create_before_destroy = true
  }
  value = remotearchive("https://raw.githubusercontent.com/pulumi/pulumi/7b0eb7fb10694da2f31c0d15edf671df843e0d4c/cmd/pulumi-test-language/tests/testdata/l2-resource-asset-archive/archive.tar")
}
// Plain (non-nested) asset/archive outputs must round-trip through stack
// outputs in every language.
output "assetOutput" {
  value = fileasset("../test.txt")
}
output "archiveOutput" {
  value = filearchive("../archive.tar")
}
// Regression test for pulumi/pulumi#16384: array and map of assets/archives
// must compose properly through Go program generation.
output "assetList" {
  value = [fileasset("../test.txt"), stringasset("file contents")]
}
output "archiveList" {
  value = [filearchive("../archive.tar"), filearchive("../folder")]
}
output "assetMap" {
  value = {
    "file"   = fileasset("../test.txt")
    "string" = stringasset("file contents")
  }
}
output "archiveMap" {
  value = {
    "tar"    = filearchive("../archive.tar")
    "folder" = filearchive("../folder")
  }
}
