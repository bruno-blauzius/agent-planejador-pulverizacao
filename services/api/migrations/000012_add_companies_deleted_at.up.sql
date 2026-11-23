-- Exclusão lógica de empresa (DELETE /companies/{id}). O CNPJ continua reservado:
-- uq_companies_document é total e NÃO ignora linhas excluídas, de propósito.
ALTER TABLE companies ADD COLUMN deleted_at timestamptz;

-- NOT VALID + VALIDATE evita manter o lock exclusivo durante a varredura (tabela existente).
ALTER TABLE companies ADD CONSTRAINT ck_companies_deleted
    CHECK (deleted_at IS NULL OR (status = 'inactive' AND NOT is_platform_operator)) NOT VALID;
ALTER TABLE companies VALIDATE CONSTRAINT ck_companies_deleted;

-- Listagem: exclui as removidas e a operadora. Parcial, então não indexa o que nunca é listado.
CREATE INDEX ix_companies_listed ON companies (name) WHERE deleted_at IS NULL AND NOT is_platform_operator;
