-- Regra do ADR 0015 (D7): a conta do realm só é apagada se foi criada por este convite e nunca usada.
-- ADD COLUMN com DEFAULT constante não reescreve a tabela (PostgreSQL 11+).
ALTER TABLE invitations ADD COLUMN account_created boolean NOT NULL DEFAULT false;
