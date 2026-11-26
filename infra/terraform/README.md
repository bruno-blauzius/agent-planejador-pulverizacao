# Terraform

Infraestrutura em nuvem do produto. Hoje cobre só o bucket S3 dos documentos de proprietário ([ADR 0020](../../docs/adr/0020-documentos-de-proprietario-no-s3.md)).

```
modules/owner-documents-bucket/   bucket privado, KMS próprio, versionamento, ciclo de vida, política TLS e política IAM da API
envs/dev/                         ambiente dev: provider, backend S3 e a instância do módulo
```

## O que o módulo cria

| Recurso | Por quê |
|---|---|
| Chave KMS com rotação anual, e alias | Documentos pessoais (RG, CNH, contrato social) criptografados com chave própria |
| Bucket com `BucketOwnerEnforced` e bloqueio de acesso público | Nenhum objeto público, nenhuma ACL |
| Versionamento, e expiração das versões antigas | Recupera exclusão acidental; versões antigas expiram depois de N dias |
| Descarte de uploads multipart incompletos | Evita custo de partes órfãs |
| Política do bucket | Recusa conexão sem TLS e gravação com outra chave KMS |
| Política IAM da API | Só `PutObject`, `GetObject` e `DeleteObject` em `companies/*`, mais `GenerateDataKey` e `Decrypt` da chave via S3. Sem listar o bucket |

O navegador nunca acessa o bucket: a API recebe o upload e entrega o download.

## Aplicar (dev)

O estado fica num bucket S3 criado à parte, uma vez por conta.

```bash
cd infra/terraform/envs/dev
cp backend.hcl.example backend.hcl            # bucket de estado, chave e região
cp terraform.tfvars.example terraform.tfvars  # name_prefix único (nome de bucket é global)
terraform init -backend-config=backend.hcl
terraform plan -out=tfplan
terraform apply tfplan
```

Credenciais vêm da cadeia padrão da AWS (`AWS_PROFILE`, SSO etc.). Nada de chave no repositório.

Depois do `apply`, preencha o `.env` da raiz com as saídas:

```bash
API_DOCUMENTS_BUCKET=$(terraform output -raw api_documents_bucket)
API_DOCUMENTS_REGION=$(terraform output -raw api_documents_region)
```

Anexe a política `api_policy_arn` à identidade com que a API roda:

- em produção, use `api_role_name` (papel da carga de trabalho);
- em dev, use um usuário ou papel do seu perfil local.

Sem `API_DOCUMENTS_BUCKET`, a API sobe normalmente, e só as rotas de documentos respondem `503`.

## Verificar sem aplicar

```bash
terraform -chdir=infra/terraform fmt -recursive -check
terraform -chdir=infra/terraform/envs/dev init -backend=false
terraform -chdir=infra/terraform/envs/dev validate
```
