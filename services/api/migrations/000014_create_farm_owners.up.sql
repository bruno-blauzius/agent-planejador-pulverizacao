-- Vínculo entre fazenda e proprietário: papel e participação. A soma das participações
-- por fazenda (100% para fazenda ativa, nunca acima de 100%) é regra de negócio e fica
-- no domínio, não aqui. "Substituir vínculos" apaga e regrava, por isso o DELETE é permitido.
CREATE TABLE farm_owners (
    company_id    uuid          NOT NULL,
    farm_id       uuid          NOT NULL,
    owner_id      uuid          NOT NULL,
    role          text          NOT NULL,
    share_percent numeric(5, 2),
    created_at    timestamptz   NOT NULL DEFAULT now(),

    -- Um vínculo por par fazenda/proprietário (um papel só), como no domínio e nas telas.
    CONSTRAINT pk_farm_owners PRIMARY KEY (farm_id, owner_id),
    -- As duas FKs compartilham company_id: o banco recusa vincular entre empresas.
    CONSTRAINT fk_farm_owners_farm FOREIGN KEY (company_id, farm_id)
        REFERENCES farms (company_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_farm_owners_owner FOREIGN KEY (company_id, owner_id)
        REFERENCES owners (company_id, id) ON DELETE RESTRICT,
    CONSTRAINT ck_farm_owners_role CHECK (role IN ('owner', 'tenant', 'legal_representative')),
    -- Só o papel owner tem participação; arrendatário e responsável legal não têm.
    CONSTRAINT ck_farm_owners_share CHECK (
        (role = 'owner' AND share_percent IS NOT NULL AND share_percent > 0 AND share_percent <= 100)
        OR (role <> 'owner' AND share_percent IS NULL)
    )
);

-- Vínculos de um proprietário (a PK já atende a busca por fazenda).
CREATE INDEX ix_farm_owners_company_owner ON farm_owners (company_id, owner_id);
