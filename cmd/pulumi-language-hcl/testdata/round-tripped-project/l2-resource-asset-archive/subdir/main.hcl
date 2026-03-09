terraform {
  required_providers {
    asset-archive = {
      source  = "pulumi/asset-archive"
      version = "5.0.0"
    }
  }
}

resource "asset-archive_assetresource" "ass" {
  value = fileAsset("../test.txt")
}
resource "asset-archive_archiveresource" "arc" {
  value = fileArchive("../archive.tar")
}
resource "asset-archive_archiveresource" "dir" {
  value = fileArchive("../folder")
}
resource "asset-archive_archiveresource" "assarc" {
  value = assetArchive({
    "string"  = stringAsset("file contents")
    "file"    = fileAsset("../test.txt")
    "folder"  = fileArchive("../folder")
    "archive" = fileArchive("../archive.tar")
  })
}
resource "asset-archive_assetresource" "remoteass" {
  value = remoteAsset("https://raw.githubusercontent.com/pulumi/pulumi/7b0eb7fb10694da2f31c0d15edf671df843e0d4c/cmd/pulumi-test-language/tests/testdata/l2-resource-asset-archive/test.txt")
}
