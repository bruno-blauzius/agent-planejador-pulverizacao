variable "region" {
  description = "Região AWS do bucket (API_DOCUMENTS_REGION). São Paulo por padrão: dado pessoal de brasileiros fica no país."
  type        = string
  default     = "sa-east-1"
}

variable "environment" {
  description = "Nome do ambiente, parte do nome do bucket."
  type        = string
  default     = "dev"
}

variable "name_prefix" {
  description = "Prefixo único do nome do bucket (nomes de bucket são globais na AWS)."
  type        = string
}

variable "noncurrent_version_retention_days" {
  description = "Dias que uma versão substituída ou apagada fica guardada."
  type        = number
  default     = 30
}

variable "api_role_name" {
  description = "Papel IAM da API que recebe a política de acesso. Nulo em dev, quando a API roda com um perfil local."
  type        = string
  default     = null
}

variable "access_log_bucket" {
  description = "Bucket de logs de acesso do S3. Nulo desliga os logs."
  type        = string
  default     = null
}
