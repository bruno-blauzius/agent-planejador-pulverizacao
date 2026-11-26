variable "bucket_name" {
  description = "Nome global do bucket dos documentos de proprietário (ex.: agentes-agro-dev-owner-documents)."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$", var.bucket_name)) && !strcontains(var.bucket_name, "..")
    error_message = "Use 3 a 63 caracteres: minúsculas, dígitos, ponto e hífen, começando e terminando com letra ou dígito."
  }
}

variable "object_prefix" {
  description = "Prefixo das chaves que a API grava (domain.NewOwnerDocument): companies/{empresa}/owners/{proprietário}/documents/{id}."
  type        = string
  default     = "companies/"
}

variable "noncurrent_version_retention_days" {
  description = "Dias que uma versão substituída ou apagada fica guardada antes de expirar (recuperação de exclusão acidental)."
  type        = number
  default     = 90

  validation {
    condition     = var.noncurrent_version_retention_days >= 1
    error_message = "Guarde as versões antigas por ao menos 1 dia."
  }
}

variable "abort_incomplete_upload_days" {
  description = "Dias até descartar uploads multipart incompletos (que ocupam espaço sem aparecer na listagem)."
  type        = number
  default     = 7
}

variable "kms_deletion_window_days" {
  description = "Janela de espera antes de apagar a chave KMS (7 a 30 dias). Sem a chave, os documentos ficam ilegíveis."
  type        = number
  default     = 30

  validation {
    condition     = var.kms_deletion_window_days >= 7 && var.kms_deletion_window_days <= 30
    error_message = "A AWS aceita de 7 a 30 dias."
  }
}

variable "api_role_name" {
  description = "Papel IAM da API que recebe a política de acesso ao bucket. Nulo: a política é criada e só exportada."
  type        = string
  default     = null
}

variable "access_log_bucket" {
  description = "Bucket que recebe os logs de acesso do S3 (auditoria). Nulo desliga os logs de acesso."
  type        = string
  default     = null
}

variable "tags" {
  description = "Tags aplicadas a todos os recursos do módulo."
  type        = map(string)
  default     = {}
}
