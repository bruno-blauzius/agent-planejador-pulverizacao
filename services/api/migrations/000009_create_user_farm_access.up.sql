-- Escopo 'selected' do operador de planejamento: quais fazendas ele enxerga.
-- A regra "selected exige ao menos uma linha" é da aplicação (mesma transação).
CREATE TABLE user_farm_access (
    user_id    uuid        NOT NULL,
    farm_id    uuid        NOT NULL,
    company_id uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT pk_user_farm_access PRIMARY KEY (user_id, farm_id),
    -- As duas FKs compartilham company_id: o banco recusa liberar fazenda de outra empresa.
    CONSTRAINT fk_user_farm_access_user FOREIGN KEY (company_id, user_id)
        REFERENCES users (company_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_user_farm_access_farm FOREIGN KEY (company_id, farm_id)
        REFERENCES farms (company_id, id) ON DELETE CASCADE
);

-- Quem enxerga a fazenda (a PK já atende a busca por usuário).
CREATE INDEX ix_user_farm_access_company_farm ON user_farm_access (company_id, farm_id);
