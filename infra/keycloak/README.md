# Keycloak: a parte do Agentes Agro no realm `inceres`

O Keycloak é o SSO compartilhado dos produtos Inceres e roda no projeto `infra-docker`
(ver o `keycloak/README.md` de lá). Esta pasta guarda **o que é do produto** dentro do realm
`inceres` e os scripts que o aplicam e o verificam. Decisões e motivos: [ADR 0015](../../docs/adr/0015-identidade-de-usuarios-e-keycloak.md).
O que depende da plataforma (e não deste repositório): [PLATFORM-REQUESTS.md](./PLATFORM-REQUESTS.md).

| Arquivo | O que é |
|---|---|
| [agentes-agro.json](./agentes-agro.json) | Estado desejado, declarativo e versionado (clients, roles, grupos, *client scopes*, requisitos do realm) |
| [apply_product.py](./apply_product.py) | Aplica o estado (`--check` compara, `--audit` audita); biblioteca padrão, sem dependências |
| [realm_checks.py](./realm_checks.py), [audit.py](./audit.py) | Checagem dos requisitos do realm e auditoria de membros (somente leitura) |
| [seed_dev_users.py](./seed_dev_users.py) | 3 usuários de desenvolvimento, um por papel (só `localhost`) |
| [verify_realm.py](./verify_realm.py) | Confere os tokens contra a tabela de permissões do `openapi.yaml` |
| [tests/](./tests) | Testes sem rede (Keycloak em memória e servidor HTTP local) |

## O que é criado

| Objeto | Nome | Para quê |
|---|---|---|
| Client | `agentes-agro` | Login do produto. Confidencial, só *Authorization Code* com PKCE S256; a `services/api` é o cliente (BFF). Sem *implicit*, sem *password grant*, `redirect_uri` exata (`<API>/auth/callback` para o login e `<API>/auth/login` para o link dos e-mails de convite), *back-channel logout* em `<API interna>/auth/backchannel-logout`, `fullScopeAllowed` desligado, *mapper* de audiência (`aud = agentes-agro`) e *mapper* explícito das roles do produto (ver abaixo) |
| Client | `agentes-agro-provisioner` | Conta de serviço da API para provisionar usuários. Escopo mínimo: `manage-users` e `view-users` de `realm-management`; token de acesso de 120 s; sem *refresh token* |
| *Client roles* de permissão (8) | `dashboard.view`, `companies.manage`, `users.manage`, `users.manage_all`, `registry.write`, `applications.register`, `applications.confirm`, `planner.use` | Capacidades que a API e o front verificam. Mesmos nomes do `Permission` do contrato |
| *Client roles* de papel (3, compostos) | `super_admin`, `admin`, `planning_operator` | Cada um inclui as permissões do perfil |
| Grupos | `/agentes-agro`, `/agentes-agro/super-admins`, `/admins`, `/planning-operators` | Unidade de atribuição: cada subgrupo mapeia **um** papel. Estar em um deles é o que libera o produto |

A tabela papel → permissões tem **uma só fonte documentada**, o `openapi.yaml` (`Permission`),
e este JSON a implementa. O teste `ContractTest` falha se os dois divergirem.

### O token do produto

Os dois clients **não** usam os *client scopes* `roles` e `web-origins` (nem o provisionador usa `acr`, `profile` e `email`).
O claim vem de um *mapper* declarado no JSON (`client-roles-agentes-agro`, `oidc-usermodel-client-role-mapper`),
só no *access token*, em `resource_access.agentes-agro.roles`. Consequência: o token **nunca** traz `realm_access` nem roles de outro client, mesmo que alguém
acrescente uma por engano ao usuário. `agentes-agro` mantém `acr`, `profile`, `basic` e `email` (nome e `preferred_username` continuam nos tokens; trocar `profile` é a decisão C6 do
[PLATFORM-REQUESTS.md](./PLATFORM-REQUESTS.md)). Claims do token de exemplo de um administrador: `acr`, `aud`, `azp`, `email`, `email_verified`, `exp`, `family_name`, `given_name`,
`iat`, `iss`, `jti`, `name`, `preferred_username`, `resource_access`, `scope`, `sid`, `sub`, `typ`.

## Como usar

Pré-requisito: o Keycloak do `infra-docker` de pé (`./up.sh` naquele projeto).

| Alvo `make` | Comando sem `make` | O que faz |
|---|---|---|
| `make keycloak` | `uv run --no-project python infra/keycloak/apply_product.py --init-secrets` | Aplica; gera no `.env` os segredos que faltarem |
| `make keycloak-check` | `uv run --no-project python infra/keycloak/apply_product.py --check` | Compara com o JSON e confere os requisitos do realm; sai com 1 se divergir |
| `make keycloak-audit` | `uv run --no-project python infra/keycloak/apply_product.py --audit` | Auditoria somente leitura dos membros; sai com 1 se houver violação |
| `make keycloak-seed` | `uv run --no-project python infra/keycloak/seed_dev_users.py` | Cria os 3 usuários de desenvolvimento (`--reset-passwords`, `--cleanup`) |
| `make keycloak-verify` | `uv run --no-project python infra/keycloak/verify_realm.py` | Confere os tokens de exemplo com o contrato |
| `make keycloak-test` | `uv run --no-project python -m unittest discover -s infra/keycloak/tests -t infra/keycloak` | Testes do script (sem rede) |
| `make keycloak-types` | `uv run --no-project --with mypy mypy --strict infra/keycloak` | Tipos estritos |

**`make` não existe no Windows** (a menos que o Git Bash ou o WSL o forneça): use a coluna do meio, que funciona em qualquer terminal. As ferramentas de lint do repositório
(`uv run --no-project --with ruff ruff check infra/keycloak` e `ruff format --check infra/keycloak`) seguem o mesmo padrão.

Variáveis (no `.env`; modelo no `.env.example`):

| Variável | Uso |
|---|---|
| `KEYCLOAK_URL` | Endereço do Keycloak (padrão `http://localhost:8180`). **Fora de `localhost`, `127.0.0.1`, `::1` e `keycloak`, exige `https`**: a credencial do admin não trafega em claro para um host remoto |
| `KEYCLOAK_REALM` | Realm (padrão: o do JSON, `inceres`). O script **recusa** o `master` |
| `KEYCLOAK_AUTH_REALM` | Realm que emite o token do admin (padrão `master`) |
| `KEYCLOAK_ADMIN_USER`, `KEYCLOAK_ADMIN_PASSWORD` | Admin do realm `master` (em desenvolvimento, o do `infra-docker`) |
| `KEYCLOAK_ADMIN_CLIENT_ID`, `KEYCLOAK_ADMIN_CLIENT_SECRET` | Alternativa para ambientes compartilhados: conta de serviço no lugar de usuário e senha |
| `KEYCLOAK_AGENTES_AGRO_CLIENT_SECRET`, `KEYCLOAK_PROVISIONER_CLIENT_SECRET` | Segredos dos clients, mínimo de 32 caracteres. `--init-secrets` gera os que faltarem |
| `AGENTES_AGRO_API_URL`, `AGENTES_AGRO_API_INTERNAL_URL`, `AGENTES_AGRO_WEB_URL` | URLs do produto. Padrão: `http://localhost:<API_HOST_PORT>`, `http://agro-api:8080` e `http://localhost:<WEB_HOST_PORT>` |
| `KEYCLOAK_SEED_PASSWORD_SUPER_ADMIN`, `_ADMIN`, `_PLANNING_OPERATOR` | Senhas dos usuários do seed (mínimo de 8 caracteres, com número e minúscula). Só no `.env` |

Para **rotacionar** um segredo, esvazie a variável no `.env` e rode `make keycloak`: o script
gera um novo e atualiza o client. A gravação do `.env` é atômica (arquivo temporário + `os.replace`) e a linha existente é
substituída no lugar, sem duplicar o nome.

## Garantias do script

- **Idempotente:** cria o que falta e corrige divergência; rodar de novo não muda nada.
- **Só o que é do produto:** recusa clients e grupos fora do prefixo `agentes-agro` e nunca toca em outro objeto do realm. Poda apenas o que pende dos objetos do produto (mappers e *client scopes* dos seus clients, mapeamentos dos seus grupos, composites dos seus papéis).
- **Nunca apaga role, grupo ou client.** Role ou subgrupo declarado que sobrou vira *aviso*. Já estes são **exatos**, e o que não está declarado é revogado ou removido:
  - mapeamentos de **grupo** (realm **e** qualquer client: um `realm-admin` acrescentado a mão é revogado), de conta de serviço e de escopo;
  - **composites** dos papéis e das permissões (qualquer role do realm ou de outro client que entre num papel do produto sai);
  - *client scopes* default e optional dos dois clients, e os *mappers* que não estão no JSON;
  - atributos de segurança dos clients: baseline do JSON mais uma lista fechada (device grant, CIBA, *token exchange*, PAR, certificado ligado, DPoP, token leve), que volta a `false` se alguém ligar.
- **Não imprime segredo nem token.** As mensagens de erro trazem só o status e o texto do Keycloak; resposta que não é JSON (uma página HTML com 200, por exemplo) vira erro limpo, e o segredo é comparado em tempo constante.
- **`--check` e `--audit` não escrevem nada**, nem no `.env`. O realm em si **nunca** é gravado pelo script: os requisitos dele (`realmRequirements`) só são conferidos.

### Requisitos do realm (`--check`)

Divergência faz o `--check` sair com 1, com mensagem clara: `registrationAllowed=false`, `verifyEmail=true`, `duplicateEmailsAllowed=false`,
`sslRequired` em `external` ou `all`, eventos e eventos de administração ligados, proteção contra força bruta e política de senha com `length(8)`, `digits` e `lowerCase`.
**Avisos** (não falham): `revokeRefreshToken=false` e `adminPermissionsEnabled=false`. Quem muda o realm é a plataforma.

### Auditoria (`--audit`)

Conta os membros de cada subgrupo e aponta **violação** (sai com 1) para usuário em mais de um subgrupo (a regra é exatamente um papel) e para usuário com role do produto atribuída direto,
fora de grupo. Conta desabilitada ou com e-mail não verificado dentro de grupo é **aviso**. A saída traz só os 8 primeiros caracteres do id e o e-mail mascarado (`a***@e***.com`).

## O que a verificação cobre e como reproduzi-la

`make keycloak-seed && make keycloak-verify` (depois `--cleanup` do seed, se não quiser os usuários). Para cada usuário seed, `verify_realm.py` pede ao Keycloak
`GET /clients/{id}/evaluate-scopes/generate-example-access-token?userId=`, sem login e sem senha, e confere:

- `resource_access.agentes-agro.roles` é **exatamente** o papel mais as permissões da tabela do `openapi.yaml`;
- `aud` inclui `agentes-agro` e `azp` é `agentes-agro`; `sub`, `email` e `email_verified` presentes;
- o token não tem `realm_access` nem roles de outros clients;
- a conta de serviço do provisionador (que não está em grupo nenhum) não recebe role do produto.

Antes disso, `make keycloak-check` garante que o Keycloak está igual ao JSON, e os testes (`make keycloak-test`, 89 casos) cobrem o reconciliador contra um Keycloak em memória,
o cliente HTTP contra um servidor local, o seed, a auditoria e a verificação.

Verificação anterior, com **login real** (*Authorization Code* + PKCE) e um usuário em cada grupo:

- `iss` público do realm, `aud` e `azp` = `agentes-agro`, `sub`, `sid` e e-mail verificado presentes;
- usuário sem grupo recebe token sem roles do produto (a API responderá `403 no_access_to_product`);
- sem PKCE, com `redirect_uri` diferente, com *password grant*, com segredo ou `code_verifier` errados: tudo recusado;
- o provisionador consegue ler grupos, criar usuário, pô-lo e tirá-lo do grupo e encerrar sessões; **não** consegue listar ou criar clients, alterar o realm nem acessar o `master`.
  Depois da poda de *client scopes*, a leitura de grupos, a mudança de grupo e o encerramento de sessões continuam funcionando, e o token dele tem 120 s de vida.

## Achados que valem para quem for criar outro produto

- **O Keycloak 26.8 exige o *client scope* `kc-admin-api`** em qualquer client que chame a Admin API. Sem ele o token é emitido, mas a API responde **401** (a audiência `…/admin/realms/<realm>` não está no token). Usuário comum com `manage-users` passa porque o `admin-cli` já tem o scope.
- **A Admin API autoriza pelos mapeamentos reais da conta de serviço**, não pelo claim do token: o provisionador funciona sem o *client scope* `roles` (o token dele nem traz `resource_access`).
- **`fullScopeAllowed` está deprecado** (aviso no log). Os clients daqui usam `false` e mapeamento de escopo explícito; as roles do próprio client sempre entram no escopo, por isso o *mapper* do produto não precisa de *scope mapping*.
- **No Windows, `localhost` resolve primeiro para `::1`**, e a porta do Docker só escuta em IPv4: cada conexão nova espera ~2 s. O script reutiliza uma conexão (de 76 s para 2,6 s); em outros scripts, prefira `127.0.0.1` ou reutilize conexão.
- **Nome e sobrenome são obrigatórios** no perfil padrão: sem eles o usuário não obtém token.

## Pendências

- Os endpoints `GET /auth/callback` e `POST /auth/backchannel-logout` da API **ainda não existem** (Fase A). Até lá, o *back-channel logout* apenas registra falha no log do Keycloak.
- Tudo que é da plataforma (rotação de refresh token, política de senha, retenção de eventos, SMTP, produção, backup, FGAP v2, MFA…) está em [PLATFORM-REQUESTS.md](./PLATFORM-REQUESTS.md).
- A conta do provisionador ainda tem o alcance de realm do `manage-users`; restringi-la aos grupos do produto depende do FGAP v2 (ADR 0015, D5, regra 3).
