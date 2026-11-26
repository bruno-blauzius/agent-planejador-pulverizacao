PYTHON_SERVICES := services/spray-planner services/label-advisor

.PHONY: hooks lint security audit up dev down build logs ps keycloak keycloak-check keycloak-test keycloak-seed keycloak-verify keycloak-audit keycloak-types migrate-up migrate-down migrate-version api-load-municipalities db-test api-test-keycloak api-test-oidc

up: ## Sobe Postgres, api, agentes e web (exige a network externa inceres-infra)
	docker compose up -d --build

keycloak: ## Aplica no Keycloak do infra-docker (realm inceres) o client, as roles e os grupos do produto
	uv run --no-project python infra/keycloak/apply_product.py --init-secrets

keycloak-check: ## Mostra se o Keycloak diverge de infra/keycloak/agentes-agro.json (sai com 1 se divergir)
	uv run --no-project python infra/keycloak/apply_product.py --check

keycloak-test: ## Testes do script de aplicação no Keycloak (sem rede)
	uv run --no-project python -m unittest discover -s infra/keycloak/tests -t infra/keycloak

keycloak-types: ## mypy --strict em infra/keycloak
	uv run --no-project --with mypy mypy --strict infra/keycloak

keycloak-seed: ## Cria 3 usuários de desenvolvimento (um por papel; só localhost; senhas do .env)
	uv run --no-project python infra/keycloak/seed_dev_users.py

keycloak-verify: ## Confere os tokens dos usuários seed com a tabela de permissões do openapi.yaml
	uv run --no-project python infra/keycloak/verify_realm.py

keycloak-audit: ## Auditoria somente leitura: quem está em cada papel (sai com 1 se houver violação)
	uv run --no-project python infra/keycloak/apply_product.py --audit

migrate-up: ## Aplica as migrações da API no banco api do Postgres local (papel api_migrator; golang-migrate em container)
	bash services/api/scripts/migrate.sh up

migrate-down: ## Desfaz a última migração da API no Postgres local (uma por vez)
	bash services/api/scripts/migrate.sh down 1

migrate-version: ## Versão atual das migrações da API no Postgres local
	bash services/api/scripts/migrate.sh version

api-load-municipalities: ## Baixa a malha municipal do IBGE e carrega no banco api local (papel api_migrator; ADR 0019)
	bash services/api/scripts/load-municipalities.sh

api-test-keycloak: ## Integração do adaptador Keycloak da API contra o Keycloak local (cria e apaga uma conta .test; exige make keycloak)
	cd services/api && go test -tags integration -count=1 -v -run TestKeycloakLifecycle ./internal/adapter/keycloak

api-test-oidc: ## Login OIDC da API contra o Keycloak real: Authorization Code + PKCE, renovação, revogação (cria usuário seed se faltar e o remove; exige make keycloak)
	cd services/api && go test -tags integration -count=1 -v -run TestRealKeycloak ./internal/adapter/oidc

db-test: ## Migrações da API em Postgres descartável: up, down, up, constraints, tenant e privilégios
	bash services/api/scripts/db-test.sh

dev: ## Como o up, mas com hot reload do agro-web (fica em primeiro plano; Ctrl+C para sair)
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build --watch

down: ## Derruba os containers e mantém o volume do banco
	docker compose down

build: ## Reconstrói as imagens de todos os serviços
	docker compose build

logs: ## Acompanha os logs de todos os serviços
	docker compose logs -f --tail=100

ps: ## Estado e saúde dos containers
	docker compose ps

hooks: ## Instala os hooks de pre-commit e pre-push
	pre-commit install --install-hooks -t pre-commit -t pre-push

lint: ## Lint, formatação e checagem de tipos de todos os serviços
	pre-commit run ruff-check --all-files
	pre-commit run ruff-format --all-files
	@for svc in $(PYTHON_SERVICES); do \
		if [ -f $$svc/pyproject.toml ]; then (cd $$svc && uv run mypy src) || exit 1; fi; \
	done
	pre-commit run golangci-lint --all-files

security: ## Segredos, SQL injection e CVEs (mesmos checks do commit/push)
	pre-commit run gitleaks --all-files
	pre-commit run bandit --all-files
	pre-commit run forbid-env-and-raw-data --all-files
	pre-commit run --hook-stage pre-push pip-audit --all-files
	pre-commit run --hook-stage pre-push govulncheck --all-files

audit: ## Audita o uv.lock de cada serviço Python (CVEs conhecidos)
	@for svc in $(PYTHON_SERVICES); do \
		if [ -f $$svc/uv.lock ]; then \
			(cd $$svc && uv export --frozen --no-hashes --no-emit-project | uvx pip-audit --strict -r /dev/stdin) || exit 1; \
		fi; \
	done

SQLC_VERSION := v1.30.0

api-test: ## Testes unitários da API (sem banco)
	cd services/api && go test ./...

api-test-integration: ## Adaptador Postgres da API em banco descartável (papel api_app; tag integration)
	bash services/api/scripts/integration-test.sh

api-sqlc: ## Regera o código sqlc do adaptador Postgres da API (versão fixada em SQLC_VERSION)
	cd services/api && go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

api-sqlc-check: ## Falha se o código gerado do sqlc divergir das queries/migrações
	cd services/api && go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) diff
