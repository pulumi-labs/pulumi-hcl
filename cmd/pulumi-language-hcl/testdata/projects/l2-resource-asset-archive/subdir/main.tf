terraform {
  required_providers {
    asset-archive = {
      source  = "pulumi/asset-archive"
      version = "5.0.0"
    }
  }
}

resource "asset-archive_asset_resource" "ass" {
  value = fileAsset("../test.txt")
}
resource "asset-archive_archive_resource" "arc" {
  value = fileArchive("../archive.tar")
}
resource "asset-archive_archive_resource" "dir" {
  value = fileArchive("../folder")
}
resource "asset-archive_archive_resource" "assarc" {
  value = assetArchive({
    "string"  = stringAsset("file contents")
    "file"    = fileAsset("../test.txt")
    "folder"  = fileArchive("../folder")
    "archive" = fileArchive("../archive.tar")
  })
}
resource "asset-archive_asset_resource" "remoteass" {
  value = remoteAsset("https://raw.githubusercontent.com/pulumi/pulumi/7b0eb7fb10694da2f31c0d15edf671df843e0d4c/cmd/pulumi-test-language/tests/testdata/l2-resource-asset-archive/test.txt")
}
resource "asset-archive_archive_resource" "remotearc" {
  value = remoteArchive("https://raw.githubusercontent.com/pulumi/pulumi/7b0eb7fb10694da2f31c0d15edf671df843e0d4c/cmd/pulumi-test-language/tests/testdata/l2-resource-asset-archive/archive.tar")
}
// Plain (non-nested) asset/archive outputs must round-trip through stack
// outputs in every language.
output "assetOutput" {
  value = fileAsset("../test.txt")
}
output "archiveOutput" {
  value = fileArchive("../archive.tar")
}
// Regression test for pulumi/pulumi#16384: array and map of assets/archives
// must compose properly through Go program generation.
output "assetList" {
  value = [fileAsset("../test.txt"), stringAsset("file contents")]
}
output "archiveList" {
  value = [fileArchive("../archive.tar"), fileArchive("../folder")]
}
output "assetMap" {
  value = {
    "file"   = fileAsset("../test.txt")
    "string" = stringAsset("file contents")
  }
}
output "archiveMap" {
  value = {
    "tar"    = fileArchive("../archive.tar")
    "folder" = fileArchive("../folder")
  }
}
