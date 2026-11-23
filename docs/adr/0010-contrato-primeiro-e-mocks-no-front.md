# ADR 0010: Contrato primeiro e mocks no front enquanto a API não implementa os cadastros

- **Status:** proposto
- **Data:** 2026-11-21

## Contexto

As telas do Figma "01 · Acesso e Cadastros" (login, fazendas, assistente de nova fazenda, proprietários, áreas e talhões) precisam ser implementadas no `web/`. A `services/api` (BFF, dona dos dados de talhões e aplicações) hoje só expõe probes de saúde e documentação: nenhum endpoint de autenticação, fazendas, talhões ou proprietários existe.

O CLAUDE.md exige que o front fale **apenas** com `services/api`, que componentes não façam HTTP e que o `api/openapi.yaml` seja o contrato. Esperar a implementação Go bloquearia a UI; inventar rotas no front quebraria a regra de contrato.

## Opções

1. **Esperar a API Go.** Nenhum risco de divergência, mas bloqueia o front por fases.
2. **Rotas Nitro falsas em `web/server/api`.** Simula HTTP de verdade, mas viola a regra de que `server/` contém só as probes e cria um segundo "BFF" que tende a sobreviver.
3. **Mock server externo gerado do OpenAPI** (Prism, MSW). Fiel ao contrato, porém adiciona dependência e processo extra só para desenvolvimento.
4. **Contrato primeiro + implementação em memória atrás de portas no front** (escolhida).

## Decisão

1. Os endpoints de cadastro são descritos **agora** no `services/api/api/openapi.yaml`, marcados com `x-status: planned` e com a frase "Ainda não implementado" na descrição. A implementação Go virá em tarefa própria, seguindo esse contrato.
2. No front, cada recurso tem uma **porta** (interface TypeScript) em `app/services/ports.ts`, com duas implementações:
   - `app/services/http/`: `$fetch` com a URL base do `runtimeConfig`, mapeando DTO (snake_case do contrato) → modelo do front;
   - `app/services/mock/`: em memória, com dados realistas do Figma e latência simulada.
3. A escolha entre as duas é feita em **um único lugar**, o plugin `app/plugins/api.ts` (raiz de composição do front), lido de `runtimeConfig.public.apiMock` (`NUXT_PUBLIC_API_MOCK=true`). As implementações são entregues via `provide` (`useNuxtApp().$api`); composables consomem a porta, nunca a implementação.
   O mock só existe em **build de desenvolvimento**: o plugin o carrega por `import()` dinâmico dentro de `if (import.meta.dev)`. Em produção, `import.meta.dev` vira `false` na compilação, o bundler descarta o ramo e o mock (com o seed e a senha de teste) não entra no bundle. Se `NUXT_PUBLIC_API_MOCK=true` chegar a um build de produção, o plugin ignora a flag, emite um aviso e usa a API real. Assim, nenhuma variável pública liga o login falso numa aplicação publicada.
4. Nenhuma dependência nova. Estado compartilhado (sessão) usa `useState` do Nuxt; Pinia continua não instalado até haver necessidade real.

## Consequências

- UI e lógica avançam em paralelo à API Go; trocar mock por HTTP é mudar uma variável de ambiente.
- Risco de divergência entre mock e contrato: o mock trabalha no nível do modelo, e o mapeamento DTO → modelo do lado HTTP é coberto por testes. Ao implementar cada endpoint no Go, remover o `x-status: planned` e validar o front contra a API real.
- O mock fica fora do bundle de produção (import dinâmico sob `import.meta.dev`). Remover `services/mock/` quando todos os endpoints existirem.
- Com SSR, o mock é instanciado por requisição no servidor e uma vez no navegador: dados criados se perdem num recarregamento completo. Aceitável para desenvolvimento.
- Validação de resposta em tempo de execução (ex.: Zod) **não** foi adotada; as respostas HTTP são tipadas por convenção num único ponto (`services/http/client.ts`). Reavaliar com ADR próprio quando a API real existir.
- Regras de negócio (soma de participação = 100%, validação de CPF/CNPJ, cálculo de área do perímetro, condição de pulverização) ficam na API. O mock as imita de forma simplificada apenas para dar retorno plausível à UI.

## Adendo (2026-11-21): cookie do token de acesso

> **Superado em 2026-11-22 pela Fase B do ADR 0015.** Com a API real, a sessão é o cookie `HttpOnly` `agro_session` definido pela própria API; `useAuthToken` e o cookie `agro_access_token` existem só no mock. O texto abaixo fica como histórico.

**Situação temporária adotada.** O token Bearer devolvido por `POST /auth/login` fica no cookie `agro_access_token`, gravado pelo front (`useAuthToken`) com:

- `expires` igual ao `expires_at` da resposta de login: o cookie some junto com o token;
- `sameSite: lax`: o cookie vai nas navegações de topo, mas não em requisições cross-site disparadas por terceiros;
- `secure` fora do modo dev; `path: /`.

O cookie é **não-httpOnly**, porque é o JavaScript do cliente que o escreve. Isso o expõe a XSS. Um 401 em `GET /auth/me` apaga o cookie.

**Alternativa recomendada quando a API existir:** a `services/api` define, ela mesma, um cookie de sessão `httpOnly; Secure; SameSite=Lax` em `POST /auth/login`, e passa a aceitá-lo além do header `Authorization`. Ela também expõe `POST /auth/logout`, que apaga o cookie, e uma proteção CSRF para os métodos de escrita (token de double-submit ou checagem de `Origin`). O front deixa então de manipular o token: `useAuthToken` sai e o SSR repassa o cookie da requisição. Essa mudança de contrato pede um ADR próprio.

## Adendo (2026-11-21): login por e-mail ou CPF e "Manter conectado"

A tela de login (Figma 6:76) pede "E-mail ou CPF" e "Manter conectado". Como `POST /auth/login` ainda está `planned`, o contrato mudou sem custo de migração: o `LoginRequest` passou a ser `{ identifier, password, remember_me }`. Com `remember_me: true`, a API devolve um `expires_at` mais longo, e o cookie do front segue esse valor. No mock, as durações são 8 h e 30 dias; na API, serão configuração.

## Adendo (2026-11-22): mock parcial com a API real

Com a sessão, os usuários, os convites, as empresas e o perfil implementados na API (ADR 0015), o mock deixou de ser tudo ou nada. `NUXT_PUBLIC_API_MOCK_PENDING=true` (só em build de desenvolvimento) usa os adaptadores HTTP e troca por mock apenas os serviços cujo contrato ainda é `planned` (fazendas, talhões, culturas, proprietários, painel, planejador, aplicações, "Hoje" e clima; lista em `services/mock/pendingServices.ts`). O mock só conhece os próprios usuários de seed, então cada chamada age como o usuário de seed que tem o mesmo papel do usuário real; os ids de empresa e fazenda reais não existem nele, e os dados dos dois lados não são consistentes entre si. É andaime: encolhe a cada endpoint implementado e sai com `services/mock/`.
