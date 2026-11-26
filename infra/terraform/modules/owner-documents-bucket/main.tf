# Bucket dos documentos de proprietário (ADR 0020). Os arquivos são documentos pessoais (RG,
# CNH, contrato social): o bucket nasce privado, criptografado com chave KMS própria, com
# versionamento contra exclusão acidental e só aceita conexões TLS. A API é a única que lê e
# grava: o navegador baixa o arquivo pela API, nunca direto do S3.

data "aws_region" "current" {}

# ---- criptografia ---------------------------------------------------------------------------

resource "aws_kms_key" "documents" {
  description             = "Criptografia dos documentos de proprietário (${var.bucket_name})"
  enable_key_rotation     = true
  deletion_window_in_days = var.kms_deletion_window_days
  tags                    = var.tags
}

resource "aws_kms_alias" "documents" {
  name          = "alias/${var.bucket_name}"
  target_key_id = aws_kms_key.documents.key_id
}

# ---- bucket ---------------------------------------------------------------------------------

resource "aws_s3_bucket" "documents" {
  bucket = var.bucket_name
  tags   = var.tags
}

# Sem ACL: o dono do bucket é dono de todo objeto (ACLs desligadas).
resource "aws_s3_bucket_ownership_controls" "documents" {
  bucket = aws_s3_bucket.documents.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_public_access_block" "documents" {
  bucket = aws_s3_bucket.documents.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "documents" {
  bucket = aws_s3_bucket.documents.id

  versioning_configuration {
    status = "Enabled"
  }
}

# Criptografia padrão: a API não manda cabeçalho de criptografia; todo objeto sai com a chave KMS.
resource "aws_s3_bucket_server_side_encryption_configuration" "documents" {
  bucket = aws_s3_bucket.documents.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.documents.arn
    }
    # Chave de bucket: menos chamadas ao KMS (custo) sem mudar a proteção.
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "documents" {
  bucket = aws_s3_bucket.documents.id

  rule {
    id     = "expire-noncurrent-versions"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = var.noncurrent_version_retention_days
    }
  }

  rule {
    id     = "abort-incomplete-uploads"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = var.abort_incomplete_upload_days
    }
  }

  # O versionamento precisa existir antes da regra de versões antigas.
  depends_on = [aws_s3_bucket_versioning.documents]
}

resource "aws_s3_bucket_logging" "documents" {
  count = var.access_log_bucket == null ? 0 : 1

  bucket        = aws_s3_bucket.documents.id
  target_bucket = var.access_log_bucket
  target_prefix = "s3/${var.bucket_name}/"
}

# Recusa qualquer acesso sem TLS e qualquer objeto gravado com outra chave KMS.
data "aws_iam_policy_document" "bucket" {
  statement {
    sid     = "DenyInsecureTransport"
    effect  = "Deny"
    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.documents.arn,
      "${aws_s3_bucket.documents.arn}/*",
    ]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }

  statement {
    sid       = "DenyOtherKmsKeys"
    effect    = "Deny"
    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.documents.arn}/*"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    # Só vale quando o cliente manda o cabeçalho; sem ele, a criptografia padrão usa a chave certa.
    condition {
      test     = "StringNotEqualsIfExists"
      variable = "s3:x-amz-server-side-encryption-aws-kms-key-id"
      values   = [aws_kms_key.documents.arn]
    }
  }
}

resource "aws_s3_bucket_policy" "documents" {
  bucket = aws_s3_bucket.documents.id
  policy = data.aws_iam_policy_document.bucket.json

  # A política só pode ser gravada depois do bloqueio de acesso público.
  depends_on = [aws_s3_bucket_public_access_block.documents]
}

# ---- acesso da API (menor privilégio) ---------------------------------------------------------

# Só o que a API faz (usecase.DocumentStorage): gravar, ler e apagar objetos sob o prefixo das
# empresas. Sem listar o bucket, sem mexer em versões, sem administrar a chave.
data "aws_iam_policy_document" "api" {
  statement {
    sid     = "OwnerDocumentObjects"
    effect  = "Allow"
    actions = ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"]
    resources = [
      "${aws_s3_bucket.documents.arn}/${var.object_prefix}*",
    ]
  }

  statement {
    sid       = "OwnerDocumentKey"
    effect    = "Allow"
    actions   = ["kms:GenerateDataKey", "kms:Decrypt"]
    resources = [aws_kms_key.documents.arn]

    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["s3.${data.aws_region.current.region}.amazonaws.com"]
    }
  }
}

resource "aws_iam_policy" "api" {
  name        = "${var.bucket_name}-api"
  description = "Acesso da API Agentes Agro aos documentos de proprietário (${var.bucket_name})."
  policy      = data.aws_iam_policy_document.api.json
  tags        = var.tags
}

resource "aws_iam_role_policy_attachment" "api" {
  count = var.api_role_name == null ? 0 : 1

  role       = var.api_role_name
  policy_arn = aws_iam_policy.api.arn
}
