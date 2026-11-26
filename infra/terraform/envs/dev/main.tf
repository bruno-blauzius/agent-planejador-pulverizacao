# Ambiente dev: bucket dos documentos de proprietário (ADR 0020).
# Aplicar: veja infra/terraform/README.md (init com backend.hcl, plan, apply).

terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # Estado remoto no S3 com trava nativa (use_lockfile). Bucket, chave e região vêm de
  # backend.hcl (copie de backend.hcl.example); nada de conta ou nome fixo no código.
  backend "s3" {}
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project     = "agentes-agro"
      Environment = var.environment
      ManagedBy   = "terraform"
      Component   = "owner-documents"
    }
  }
}

module "owner_documents" {
  source = "../../modules/owner-documents-bucket"

  bucket_name                       = "${var.name_prefix}-${var.environment}-owner-documents"
  noncurrent_version_retention_days = var.noncurrent_version_retention_days
  api_role_name                     = var.api_role_name
  access_log_bucket                 = var.access_log_bucket
}
