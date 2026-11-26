output "api_documents_bucket" {
  description = "Valor de API_DOCUMENTS_BUCKET no .env."
  value       = module.owner_documents.bucket_name
}

output "api_documents_region" {
  description = "Valor de API_DOCUMENTS_REGION no .env."
  value       = module.owner_documents.region
}

output "kms_key_arn" {
  description = "Chave KMS dos documentos."
  value       = module.owner_documents.kms_key_arn
}

output "api_policy_arn" {
  description = "Política IAM da API: anexe ao papel ou usuário com que a API roda."
  value       = module.owner_documents.api_policy_arn
}
