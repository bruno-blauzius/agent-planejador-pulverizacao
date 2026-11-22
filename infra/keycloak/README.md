# Keycloak: a parte do Agentes Agro no realm `inceres`

O Keycloak é o SSO compartilhado dos produtos Inceres e roda no projeto `infra-docker`
(ver o `keycloak/README.md` de lá). Esta pasta guarda **o que é do produto** dentro do realm
`inceres` e o script que o aplica. Decisões e motivos: [ADR 0015](../../docs/adr/0015-identidade-de-usuarios-e-keycloak.md).

| Arquivo | O que é |
|---|---|
| [agentes-agro.json](./agentes-agro.json) | Estado desejado, declarativo e versionado |
| [apply_product.py](./apply_product.py) | Aplica o estado no Keycloak (biblioteca padrão, sem dependências) |
| [tests/](./tests) | Testes do script, sem rede (Keycloak em memória e servidor HTTP local) |

## O que é criado

| Objeto | Nome | Para quê |
|---|---|---|
| Client | `agentes-agro` | Login do produto. Confidencial, só *Authorization Code* com PKCE S256; a `services/api` é o cliente (BFF). Sem *implicit*, sem *password grant*, `redirect_uri` exata (`<API>/auth/callback`), *back-channel logout* em `<API interna>/auth/backchannel-logout`, `fullScopeAllowed` desligado e um *mapper* de audiência (`aud = agentes-agro`) |
| Client | `agentes-agro-provisioner` | Conta de serviço da API para provisionar usuários. Escopo mínimo: `manage-users` e `view-users` de `realm-management`, mais o *client scope* `kc-admin-api` |
| *Client roles* de permissão (8) | `dashboard.view`, `companies.manage`, `users.manage`, `users.manage_all`, `registry.write`, `applications.register`, `applications.confirm`, `planner.use` | Capacidades que a API e o front verificam. Mesmos nomes do `Permission` do contrato |
| *Client roles* de papel (3, compostos) | `super_admin`, `admin`, `planning_operator` | Cada um inclui as permissões do perfil |
| Grupos | `/agentes-agro`, `/agentes-agro/super-admins`, `/admins`, `/planning-operators` | Unidade de atribuição: cada subgrupo mapeia **um** papel. Estar em um deles é o que libera o produto |

A tabela papel → permissões tem **uma só fonte documentada**, o `openapi.yaml` (`Permission`),
e este JSON a implementa. O teste `ContractTest` falha se os dois divergirem.

## Como usar

Pré-requisito: o Keycloak do `infra-docker` de pé (`./up.sh` naquele projeto).

```bash
make keycloak         # aplica; gera no .env os segredos que faltarem
make keycloak-check   # só compara; sai com 1 se o Keycloak divergir do JSON
make keycloak-test    # testes do script (sem rede)
```

Sem `make`: `uv run --no-project python infra/keycloak/apply_product.py [--check] [--init-secrets]`.

Variáveis (no `.env`; modelo no `.env.example`):

| Variável | Uso |
|---|---|
| `KEYCLOAK_URL` | Endereço do Keycloak (padrão `http://localhost:8180`) |
| `KEYCLOAK_REALM` | Realm (padrão: o do JSON, `inceres`). O script **recusa** o `master` |
| `KEYCLOAK_ADMIN_USER`, `KEYCLOAK_ADMIN_PASSWORD` | Admin do realm `master` (em desenvolvimento, o do `infra-docker`) |
| `KEYCLOAK_ADMIN_CLIENT_ID`, `KEYCLOAK_ADMIN_CLIENT_SECRET` | Alternativa para ambientes compartilhados: conta de serviço no lugar de usuário e senha |
| `KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET`, `KEYCLOAK_PROVISIONER_CLIENT_SECRET` | Segredos dos clients, mínimo de 32 caracteres. `--init-secrets` gera os que faltarem |
| `AGENTES_AGRO_API_URL`, `AGENTES_AGRO_API_INTERNAL_URL`, `AGENTES_AGRO_WEB_URL` | URLs do produto. Padrão: `http://localhost:<API_HOST_PORT>`, `http://agro-api:8080` e `http://localhost:<WEB_HOST_PORT>` |

Para **rotacionar** um segredo, esvazie a variável no `.env` e rode `make keycloak`: o script
gera um novo e atualiza o client.

## Garantias do script

- **Idempotente:** cria o que falta e corrige divergência; rodar de novo não muda nada.
- **Só o que é do produto:** recusa clients e grupos fora do prefixo `agentes-agro` e nunca toca em outro objeto do realm.
- **Nunca apaga.** Objeto declarado que sobrou (uma role ou um subgrupo que ninguém declarou) vira *aviso*, não remoção. Já as roles de papel, os mapeamentos de grupo, de conta de serviço e de escopo são **exatos**: o que não está declarado é revogado.
- **Não imprime segredo nem token.** As mensagens de erro trazem só o status e o texto do Keycloak.
- **`--check` não escreve nada**, nem no `.env`.

## O que foi verificado no Keycloak real

Login real (*Authorization Code* + PKCE) com um usuário em cada grupo:

- as roles do token são exatamente o papel e as permissões da tabela do contrato, nos três perfis;
- `iss` público do realm, `aud` e `azp` = `agentes-agro`, `sub`, `sid` e e-mail verificado presentes;
- o token **não** traz roles de outros clients nem do realm;
- usuário sem grupo recebe token sem roles do produto (a API responderá `403 no_access_to_product`);
- sem PKCE, com `redirect_uri` diferente, com *password grant*, com segredo ou `code_verifier` errados: tudo recusado;
- o provisionador consegue ler grupos, criar usuário, pô-lo e tirá-lo do grupo e encerrar sessões; **não** consegue listar ou criar clients, alterar o realm nem acessar o `master`, e só enxerga a representação resumida do realm.

## Achados que valem para quem for criar outro produto

- **O Keycloak 26.8 exige o *client scope* `kc-admin-api`** em qualquer client que chame a Admin API. Sem ele o token é emitido, mas a API responde **401** (a audiência `…/admin/realms/<realm>` não está no token). Usuário comum com `manage-users` passa porque o `admin-cli` já tem o scope.
- **`fullScopeAllowed` está deprecado** (aviso no log). Os clients daqui usam `false` e mapeamento de escopo explícito.
- **No Windows, `localhost` resolve primeiro para `::1`**, e a porta do Docker só escuta em IPv4: cada conexão nova espera ~2 s. O script reutiliza uma conexão (de 76 s para 2,6 s); em outros scripts, prefira `127.0.0.1` ou reutilize conexão.
- **Nome e sobrenome são obrigatórios** no perfil padrão: sem eles o usuário não obtém token.

## Pendências

- Os endpoints `GET /auth/callback` e `POST /auth/backchannel-logout` da API **ainda não existem** (Fase A). Até lá, o *back-channel logout* apenas registra falha no log do Keycloak.
- **Rotação de refresh token (`revokeRefreshToken`) está desligada** no realm. O desenho da sessão (ADR 0015, D6) a assume; ligar é decisão da plataforma, pois vale para todos os produtos.
- Restringir a conta do provisionador aos grupos do produto com as permissões administrativas finas (FGAP v2), em vez do alcance de realm do `manage-users` (ADR 0015, D5, regra 3).
