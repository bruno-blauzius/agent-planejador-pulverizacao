# ADR 0007: Pre-commit para qualidade e segurança

- **Status:** proposto
- **Data:** 2026-11-21

## Contexto

O repositório mistura Python (agentes) e Go (API) e lida com chaves de LLM. As regras do CLAUDE.md (regra de dependência, sem exceção silenciada, sem segredo no código, sem SQL montado por string) dependiam só de revisão manual. Também não havia `.gitignore`, e o `.env` com chaves reais estava a um `git add .` de ser commitado.

## Opções

1. **Só CI.** Pega o problema tarde: o segredo já está no histórico.
2. **Pre-commit com hooks gerenciados** (escolhida). Falha antes do commit e usa repositórios oficiais com versão fixada.
3. **Pre-commit com binários locais (`language: system`).** Mais controle de versão, mas cada pessoa precisa instalar tudo à mão no Windows.

## Decisão

Adotar o framework `pre-commit` com hooks em dois estágios:

| Objetivo | Python | Go |
|---|---|---|
| Saúde e padrões | `ruff` (lint e format); `mypy` via `make lint` | `golangci-lint` (govet, staticcheck, errcheck, errorlint, revive, gofmt) |
| SQL injection | `ruff` S608, `bandit` B608 | `gosec` G201/G202, `sqlclosecheck` |
| Segredo fixo | `ruff` S105-S107, `bandit`, `gitleaks` | `gosec` G101, `gitleaks` |
| Dependências vulneráveis | `pip-audit` | `govulncheck` |
| Regra de dependência | `ruff` TID251 proíbe frameworks fora de `adapters/`, `main.py`, testes e evals | pacote `usecase` declara interfaces (revisão) |

Também: `detect-private-key`, bloqueio de `.env` e de `data/raw/`, `.gitignore` e `.gitleaks.toml`.

`pip-audit` e `govulncheck` usam rede e são mais lentos. Rodam só quando `requirements*.txt`, `go.mod` ou `go.sum` mudam, e no `pre-push`. O `uv.lock` é auditado por `make audit`, e `mypy` roda em `make lint`, porque precisam do ambiente do serviço (`uv run`).

## Consequências

- Commits que violam as regras são recusados localmente. O `--no-verify` continua possível, então os mesmos checks devem rodar na CI quando ela existir.
- Os hooks Go só funcionam quando `services/api/go.mod` existir.
- Novas dependências de desenvolvimento: `pre-commit`, `ruff`, `bandit`, `pip-audit`, `golangci-lint`, `govulncheck`, `gitleaks`. Nenhuma entra em produção.
- Falsos positivos se tratam em `.gitleaks.toml`, `bandit.yaml`, `ruff.toml` ou `.golangci.yml`, com justificativa. Nunca desligando a regra para fazer o código passar.
