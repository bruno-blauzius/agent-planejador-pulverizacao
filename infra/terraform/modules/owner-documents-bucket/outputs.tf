output "bucket_name" {
  description = "Valor de API_DOCUMENTS_BUCKET."
  value       = aws_s3_bucket.documents.id
}

output "bucket_arn" {
  description = "ARN do bucket."
  value       = aws_s3_bucket.documents.arn
}

output "region" {
  description = "Valor de API_DOCUMENTS_REGION."
  value       = aws_s3_bucket.documents.region
}

output "kms_key_arn" {
  description = "Chave KMS que criptografa os documentos."
  value       = aws_kms_key.documents.arn
}

output "api_policy_arn" {
  description = "Política IAM de menor privilégio para o papel da API."
  value       = aws_iam_policy.api.arn
}
