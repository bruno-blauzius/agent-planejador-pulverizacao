PYTHON_SERVICES := services/spray-planner services/label-advisor

.PHONY: hooks lint security audit

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
