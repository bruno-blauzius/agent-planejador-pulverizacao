# ADR 0016: Protocolo do Planejador de Pulverização entre front, API e agente

- **Status:** aceito (2026-11-22)
- **Data:** 2026-11-22

## Contexto

A página do Figma "02 · Planejador de Pulverização" traz três telas: o Planejador (chat com o agente, plano sugerido por talhão, mapa de janelas de 5 dias e condições hora a hora), a confirmação humana antes de registrar a aplicação e a visão mobile do operador ("Hoje").

O CLAUDE.md fixa a arquitetura: a `services/api` (Go) é o BFF e a dona dos dados; o agente `spray-planner` (Python) fica **atrás** dela; o front fala apenas com a API. Aptidão de hora e janela (Delta T, vento, chuva, carência, intervalo, capacidade operacional) é regra determinística calculada no `domain` do `spray-planner` e nunca no front, nem no prompt. Ferramentas que gravam dados exigem aprovação humana. Falhas externas geram resposta honesta.

Nenhum endpoint do Planejador existe. Pelo ADR 0010, o contrato é descrito primeiro (`x-status: planned`) e o front usa mocks em memória enquanto o Go não implementa.

## Opções

**Resposta do chat**

1. **Streaming (SSE ou WebSocket) token a token.** Melhor percepção de latência, mas exige transporte novo no BFF, tratamento de reconexão e de interrupção no front, e o plano estruturado só existe no fim.
2. **Polling de uma tarefa assíncrona** (`POST` devolve `202` + `GET` do resultado). Desacopla do tempo do agente, mas cria estado de tarefa e endpoints extras.
3. **Resposta única** (`POST` devolve a resposta completa) (escolhida).

**Onde fica a aptidão**

1. Front recebe a previsão e calcula. Rejeitada: viola o CLAUDE.md.
2. API devolve a condição já calculada por hora e por janela (escolhida).

## Decisão

### Princípios

- O front envia **intenção e contexto**; a API (que orquestra o agente) devolve **texto, fontes, plano estruturado e condições prontas**.
- Toda condição (`suitable | attention | unsuitable | rain`), duração, alerta de janela curta, checagem e ranges ideais chegam prontos. O front só formata (horário, "3h15", "38,6 ha") e escolhe cor/ícone.
- Resposta **única** para o chat. A API é síncrona do ponto de vista do front, com timeout do cliente de 60 s. Streaming pode entrar depois como `Accept: text/event-stream` no mesmo `POST /planner/messages`, sem quebrar este contrato (o corpo final continua igual). Polling não é necessário agora.
- Todas as rotas exigem Bearer, aplicam a empresa (tenant, ADR 0013) e, para o operador, o `farm_scope`. Fazenda fora do escopo responde `403 out_of_scope`; de outra empresa, `404 not_found`.

### Endpoints (todos `x-status: planned`, tag `Planner`)

| Método e rota | Capacidade | Função |
|---|---|---|
| `POST /planner/messages` | `planner.use` | Uma mensagem do usuário com contexto (fazenda, talhões, produto) e a resposta do agente |
| `GET /planner/window-map` | `planner.use` | Mapa de janelas: condição por talhão, dia e hora (5 dias) e janelas agrupadas |
| `GET /planner/conditions` | `planner.use` | Condições hora a hora de um dia e resumo da janela escolhida |
| `POST /planner/plans/{planId}/entries/{fieldId}/review` | `applications.register` | Revisão que alimenta o modal de confirmação (checagens recalculadas agora) |
| `POST /applications` | `applications.register` | Registro da aplicação, **só com confirmação humana explícita** |
| `GET /planner/today` | `planner.use` | Visão "Hoje" do operador (status agora, métricas, linha do dia, tarefa) |
| `POST /planner/today/tasks/{taskId}/start` | `applications.register` | "Iniciar aplicação" |

### Conversa

Pedido (`PlannerMessageRequest`): `conversation_id` (nulo na primeira mensagem), `text` e `context { farm_id, field_ids[], product_kind }`. O `conversation_id` devolvido identifica a conversa (thread do agente) e viaja nas mensagens seguintes. O **histórico não é persistido pelo front nem pela API nesta fase**: o front o mantém em memória da sessão (`useState`).

Resposta (`PlannerReply`): `conversation_id`, `message { id, text, created_at }` (parágrafos separados por linha em branco), `consulted[]` ("O agente consultou": `kind`, `title`, `detail`), `plan` (`SprayPlan` ou nulo quando a resposta é só explicativa) e `suggestions[]` (próximas perguntas).

### Plano por talhão

`SprayPlan`: `id`, `farm_id`, `product_kind`, `headline` ("2 janelas aptas cobrem os 90 ha"), `total_area_ha`, `capacity_ha_per_hour` e `entries[]`. Cada entrada: talhão (`field_id`, `field_label`), `area_ha`, `window { date, start_time, end_time, condition }`, `duration_minutes` e `alert` (nulo, ou `{ kind: window_too_short | rain_risk | interval_not_met, message, required_minutes, available_minutes }`). A mensagem de alerta (incluindo a sugestão "começar 05:40") é do agente/API.

### Registro somente após confirmação humana

O plano pertence a **uma fazenda e a uma conversa**. O front só oferece "Revisar e registrar" para o plano da conversa atual e da fazenda selecionada: trocar de fazenda, trocar de usuário ou sair limpa a conversa (e uma resposta que chega depois da troca é descartada) e fecha o modal. A API não confia nisso: `review` e `POST /applications` de plano de outra empresa respondem `404`, e de fazenda fora do `farm_scope`, `403 out_of_scope`.

1. O front pede a **revisão** (`POST .../review`). A API recalcula as checagens com a previsão atual e devolve `ApplicationReview`: dados do talhão, janela, produto (dose com unidade e taxa de calda), volume de calda e tanques, `checks[]` (`code`, `status: ok | warning | blocked`, `title`, `detail`), operador, equipamento, `can_confirm` e `expires_at`. Esse retrato tem `id`. O front confere `expires_at` ao abrir e ao enviar (sem timer); a validade que vale é a da API.
2. O usuário marca "Revisei as condições e confirmo a aplicação" e confirma.
3. O front envia `POST /applications` com `plan_id`, `field_id`, `review_id` e `confirmation { confirmed: true, acknowledged_warnings[] }`. A API **valida de novo**: `confirmed` deve ser `true` (senão `422`), a revisão deve existir e não ter expirado, nenhuma checagem pode estar `blocked`, e todo `warning` precisa estar em `acknowledged_warnings`. Se as condições mudaram desde a revisão, responde `409 conditions_changed` e o front reabre a revisão. Registro repetido responde `409 conflict`.
4. Sucesso: `201` com a aplicação (`status: planned`). O agente nunca grava por conta própria: a ferramenta de registro do `spray-planner` só produz a proposta; quem chama `POST /applications` é o usuário.

### Visão do operador ("Hoje")

`GET /planner/today` devolve a fazenda, `as_of`, `status` (`condition`, `headline` "Pode pulverizar", `until_time`, `remaining_minutes`, janela), `metrics[]` (Delta T, vento, chuva, cada um com valor, unidade, rótulo e `condition`), `hours[]` (condição por hora e `is_current`) e `task` (aplicação planejada de hoje, ou nula). A tarefa é uma aplicação **já registrada** (`planned`); "Iniciar aplicação" a passa para `in_progress` (`409` se já iniciada ou se a janela fechou). O operador só vê tarefas das fazendas do seu escopo.

### Histórico de aplicações (telas 19 e 20)

Extensão do mesmo protocolo, também `x-status: planned`:

| Método e rota | Capacidade | Função |
|---|---|---|
| `GET /applications` | nenhuma além de estar autenticado e no escopo | Lista paginada (histórico e planejadas) |
| `GET /applications/{applicationId}` | idem | Detalhe (painel lateral) |
| `GET /weather/current` | idem | Chip de clima do topo ("Guaxupé · 24 °C · vento 7 km/h") |

- **Status:** `planned` (registrada após a confirmação humana, ainda não executada), `in_progress` (iniciada pelo operador), `completed` e `cancelled`. O registro da tela 17 (`RegisteredApplication`) nasce `planned` e passa a aparecer na lista; "Iniciar aplicação" (tela 18) a leva a `in_progress`.
- **Escopo:** a API devolve só aplicações da empresa do usuário e das fazendas do seu `farm_scope`. `farm_id` de outra empresa responde `404`; de fazenda fora do escopo, `403 out_of_scope`. O mesmo vale para o detalhe. Ler não exige capacidade (como os cadastros, ADR 0013); por isso **não há permissão nova** (a tabela do `openapi.yaml` e o Keycloak não mudam). O operador vê as aplicações das fazendas liberadas para ele, não só as suas (ver decisões em aberto).
- **Filtros:** `period_days` (7, 30 ou 90; padrão 30) conta a partir de hoje menos N dias e **inclui as planejadas futuras**; `farm_id`, `field_id`, `product_kind`, `operator_id`, `status`, `page` e `page_size`. A ordem é da API (planejadas e em andamento primeiro, a mais próxima antes; depois as passadas, da mais recente para a mais antiga). A resposta traz `facets` (talhões e operadores do escopo, sem aplicar os filtros) para alimentar os selects.
- **Indicadores do topo (`summary`):** a resposta da lista traz os 4 cartões do mês corrente no escopo do usuário (e em `farm_id`, se dado): `month_count` (total e por status), `applied_area` (ha, área produtiva e percentual), `within_window` (percentual, quantas e de quantas com Delta T e vento ideais) e `next` (próxima planejada: talhão, data, janela e produto). Tudo calculado pela API; **não muda** com `period_days`, talhão, produto, operador, status nem página. Valores ausentes chegam `null` e o front mostra "—".
- **Linha da lista:** janela, talhão (código, nome, variedade, bloco), produto (tipo, dose com unidade, calda), área, condições, operador e status. `conditions.source` diz se os números foram `recorded` (medidos), `forecast` (previsão para a janela) ou `live` (sem números: "Previsão ao vivo"); `rain_mm` numa `cancelled` é o motivo "Chuva 18 mm". O front só escolhe o texto pelo que veio.
- **Detalhe:** `registered_at`, `recorded_conditions` (veredito "Dentro da janela", Delta T, vento, chuva e horizonte, duração), `usage` (lote, dose total, calda total e tanques, **calculados pela API**), `pre_harvest` (dias e "colheita liberada a partir de"), `equipment`, `confirmed_by` (pessoa e instante), `signed` e `cancellation`. Nada é recalculado no front.
- **Ações sem endpoint (desabilitadas "Em breve"):** "Exportar caderno de campo" e "Baixar registro (PDF)". "Planejar repetição" não chama a API: preenche a barra do Planejador (fazenda, talhão, produto) e abre `/planner`; quem propõe é o agente e quem registra é o usuário, com a confirmação da tela 17.
- **`ProductKind`** ganha `foliar` ("Foliar"), usado no histórico do Figma.

### Erros honestos

Além dos códigos existentes, a API passa a devolver:

- `502 agent_unavailable`: o agente não respondeu ou falhou. O front mostra "O agente está indisponível" e permite tentar de novo; **nunca** exibe uma resposta inventada.
- `502 weather_unavailable`: a previsão (Open-Meteo) está fora do ar. Vale para mensagens, mapa, condições e "Hoje".
- `409 conditions_changed`: as condições mudaram entre a revisão e o registro.
- `403 forbidden` / `403 out_of_scope` (ADR 0013) e `422 validation_failed` (ex.: nenhum talhão selecionado, confirmação ausente).

### Permissões

`planner.use` abre o Planejador, o chat, o mapa, as condições e "Hoje". `applications.register` permite revisar e registrar (administrador e operador). `applications.confirm` (administrador) fica reservada à confirmação posterior de uma aplicação registrada (ver decisões em aberto) e **não** habilita o botão do modal. Menus e botões usam `can()`; a API aplica a autorização e o escopo de fazendas em toda requisição.

### No front

- `app/types/planner.ts` (modelos) e `app/types/plannerDto.ts` (DTOs); portas `PlannerService`, `ApplicationService` e `TodayService` em `services/ports.ts`; adaptadores HTTP e mock seguem o ADR 0010 (mock só por `import()` dinâmico em dev).
- Composables: `usePlannerContext`, `usePlannerChat`, `usePlannerWindows`, `useApplicationConfirmation`, `useTodayTask`. Componentes não fazem HTTP.
- O mock do Planejador **não usa LLM**: responde por regras sobre um cenário seed fiel ao Figma (talhões T3 e T7, fungicida, janela de 06h às 10h, 90 ha, 12 ha/h), com datas relativas a "hoje".

## Consequências

**Positivas**
- O front não conhece regra agronômica; trocar limiares é mudança no `spray-planner` e na configuração.
- Resposta única mantém a API Go simples (`net/http`), testável com respostas gravadas; o contrato do corpo não muda se um dia houver streaming.
- A confirmação humana é verificável no servidor (revisão + `confirmed: true` + avisos reconhecidos), não só na UI.

**Negativas / custos**
- Latência do agente aparece inteira como "carregando": a UI precisa de bom estado de espera e de timeout.
- Sem histórico persistido, recarregar a página perde a conversa (aceitável nesta fase).
- O mock simula regras do agente de forma simplificada; divergências só aparecem com a API real.

## Decisões em aberto

1. **Histórico de conversas:** persistir na API (`GET /planner/conversations/{id}`)? Hoje fica só em memória no front.
2. **`applications.confirm`:** hoje o modal só exige `applications.register`. Se o administrador precisar aprovar o que o operador registrou, falta um `POST /applications/{id}/confirm` e um status `awaiting_approval`.
3. **Mapa de janelas por talhão:** a previsão é a mesma para a fazenda inteira; a diferença entre talhões vem de cultura, carência e intervalo. Confirmar se o mapa deve refletir essas regras por talhão (contrato já permite).
4. **Fuso das horas:** as horas e datas chegam no fuso da fazenda; o front ainda usa `America/Sao_Paulo` fixo (pendência do Painel).
5. **Área aplicada do T7:** o Figma mostra 51,4 ha no plano e o cadastro seed tem 55 ha; o plano usa a "área a aplicar" (pode ser menor que a do talhão). Confirmar a semântica com o agente.
6. **Tela "Hoje" como rota própria (`/today`)** ou layout responsivo do Planejador (padrão assumido: rota própria).
7. **Operador e equipamento:** de onde vêm no registro (cadastro de equipamentos não existe); hoje a API os resolve na revisão.
8. **Timeout e retentativa** do `POST /planner/messages` (60 s assumido).
9. **Operador no histórico:** hoje vê as aplicações das fazendas liberadas; restringir às que ele executa exigiria `operator_id` implícito no escopo.
10. **Operador sem usuário:** o filtro e a coluna usam `PersonRef` (id e nome); quem executa nem sempre é usuário da plataforma (não há cadastro de operadores).
11. **Exportar caderno de campo e baixar registro (PDF):** sem endpoint; definir formato e assinatura.
12. **Cancelar e concluir aplicação:** sem endpoint no front; `completed` e `cancelled` só aparecem como dado vindo da API.
14. **Escopo dos indicadores:** o `summary` é do mês corrente; confirmar se a tela deve oferecer outro recorte (ex.: seguir o período do filtro). No mock, "No mês", "Área aplicada" e "Dentro da janela" somam à semente 6 aplicações concluídas e 265,9 ha de antes do período listado, para chegar aos números do Figma (12, 412 ha, 92%).
13. **`GET /weather/current`:** serve o chip de todas as telas; confirmar com o `spray-planner` se fica no BFF ou vem do cache da previsão.
