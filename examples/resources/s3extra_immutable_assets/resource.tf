terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "3.61.0"
    }
  }
}

locals {
  tags = {
    Hello = "World!"
  }
}

resource "aws_s3_bucket" "example" {
  bucket = "my-tf-test-bucket"
  acl    = "private"

  tags = local.tags
}

resource "s3extra_immutable_assets" "example" {
  bucket = aws_s3_bucket.example.id
  prefix = "_next/static"

  # Relative to the module path.
  glob = "**/*.{js,json}"

  file_configuration = {
    cache_control = "max-age=365000000, immutable"
  }

  tags = local.tags
}