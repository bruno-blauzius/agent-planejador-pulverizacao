# ADR 0020: Documentos de proprietário no Amazon S3

- **Status:** aceito
- **Data:** 2026-11-23

## Contexto

Proprietário precisa de documento com foto (pessoa física) ou contrato social (pessoa jurídica) para assinar o registro de aplicações. O contrato já tinha `documents` e `has_pending_documents`, e o status `pending` da fazenda, mas não havia onde guardar os arquivos. Por isso `documents` era sempre vazio e nenhuma fazenda ficava pendente. O DER (pendência 2) pressupunha armazenamento de objetos compatível com S3.

## Opções

1. **Postgres (`bytea`).** Sem infraestrutura nova, mas põe arquivos de até 10 MB no banco transacional (backup, replicação e memória do pool).
2. **MinIO local mais SDK S3 genérico.** Serve para desenvolvimento, mas não é o destino de produção.
3. **Amazon S3, com o bucket em Terraform** (escolhida).

## Decisão

- **Armazenamento:** Amazon S3. O bucket é criado pelo módulo `infra/terraform/modules/owner-documents-bucket`:
  - acesso público bloqueado e `BucketOwnerEnforced` (sem ACL);
  - SSE-KMS com chave própria, rotação anual e chave de bucket;
  - versionamento, com expiração das versões antigas e descarte de uploads incompletos;
  - política que recusa conexão sem TLS e gravação com outra chave KMS;
  - política IAM de menor privilégio para a API: Put, Get e Delete só em `companies/*`, mais `GenerateDataKey` e `Decrypt` via S3. A API não lista o bucket.
  - Região padrão `sa-east-1`: dado pessoal de brasileiros fica no país.
- **Dependência nova:** `aws-sdk-go-v2` (`config` e `service/s3`), versões de 06/11/2026, no adaptador `internal/adapter/objectstorage`. As credenciais vêm da cadeia padrão da AWS (papel da carga de trabalho em produção, perfil ou SSO em dev); nada secreto em configuração.
  - `API_DOCUMENTS_S3_ENDPOINT` existe só para apontar um emulador.
  - Sem `API_DOCUMENTS_BUCKET`, a API sobe e as rotas de documentos respondem 503 `document_storage_unavailable`.
- **Metadados no banco** (migração `000018`): tabela `owner_documents` com tipo, nome do arquivo, tipo de mídia, tamanho, `storage_key`, quem enviou e `deleted_at`. A chave é sempre `companies/{empresa}/owners/{proprietário}/documents/{id}`; um `CHECK` garante isso, e o nome enviado pelo cliente nunca entra na chave.
- **Endpoints:**
  - `POST /owners/{id}/documents` recebe multipart com `kind` e `file`. O tipo é conferido pelo conteúdo: PDF, JPEG ou PNG, senão 415. O limite é `API_OWNER_DOCUMENT_MAX_BYTES` (10 MB), acima dele 413.
  - `GET /owners/{id}/documents/{docId}/content` entrega o arquivo pela API, com `attachment` e `nosniff`; o navegador nunca fala com o S3.
  - `DELETE /owners/{id}/documents/{docId}` faz exclusão lógica; o objeto fica sob o versionamento e o ciclo de vida do bucket.
- **Consistência:** o envio grava o objeto antes e registra o metadado depois, numa transação que trava o proprietário. Se o registro falhar, o objeto é apagado. Se até a remoção falhar, os dois erros são devolvidos para reconciliação manual, nunca engolidos.
- **Pendência:** `owners.documents_pending` guarda o resultado de `domain.RequiredDocumentsMissing`, recalculado a cada envio ou remoção. A regra fica no domínio: foto (`id_card` ou `driver_license`) para pessoa física, `articles_of_incorporation` para pessoa jurídica. Uma fazenda `active` com algum vinculado pendente é exibida como `pending` (`domain.DisplayStatus`), e o filtro `status` separa `active` de `pending`.

## Consequências

**Positivas**
- Arquivos pessoais fora do banco, criptografados com chave própria, com histórico de versões e acesso mínimo.
- A pendência de documentos passa a ser real nas telas de proprietários e fazendas.

**Negativas / custos**
- Dependência nova (SDK da AWS) e infraestrutura em nuvem para a funcionalidade completa em dev. O `terraform apply` e as credenciais ficam com o time.
- **Mudança de comportamento:** todo proprietário existente nasce pendente, e toda fazenda ativa passa a aparecer como `pending` até receber os documentos.
- Downloads passam pela API e consomem banda dela. URLs pré-assinadas são a evolução se o volume crescer, e mudam o contrato.
- O teste do adaptador usa um S3 simulado em `httptest`. O teste contra o bucket real fica manual, depois do `apply`.
