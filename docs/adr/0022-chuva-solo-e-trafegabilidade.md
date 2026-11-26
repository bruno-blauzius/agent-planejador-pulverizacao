# ADR 0022: Chuva, estado do solo e trafegabilidade do talhão

- **Status:** proposto (2026-11-26); fases 0, 1 e 2 implementadas
- **Data:** 2026-11-26
- **Telas:** Figma `MeWDfQIHok0s80GbqCKGV3`, frame "04f · Fazenda · Terreno · chuva e solo do talhão", seção "Relevo dos talhões · proposta" da página `1:4`
- **Documentos de origem:** "Cruzamento chuva × solo × relevo → método de aplicação" e o "Plano de ação · back e front" (Claude Docs, 2026-11-26)

## Contexto

O relevo (ADR 0021) diz se a máquina consegue andar no talhão quanto à declividade. Falta o outro lado da mesma pergunta: **o solo aguenta a máquina hoje?** Depois de chuva forte, trator, autopropelido e turbopulverizador atolam, compactam o solo e abrem sulcos, mesmo num talhão plano. Drone e avião não dependem do solo.

Hoje nenhum serviço guarda chuva medida nem textura do solo, e o `spray-planner` ainda é só o esqueleto (não existe `FindSprayWindows`). Sem uma regra única, a tela e o agente acabariam calculando o estado do solo cada um do seu jeito.

Há duas leituras da mesma chuva que não podem se misturar:

| Regra | Pergunta | Olha para |
|---|---|---|
| Trafegabilidade do solo | A máquina entra no talhão? | Chuva **passada** (e previsão, para projetar a liberação) |
| `RainRule` do `spray-planner` | O produto vai ser lavado? | Chuva **futura**, nas horas após a aplicação |

## Opções

### Quem calcula o estado do solo

1. **O domínio da API em Go** (escolhida). A API já é dona do talhão e do relevo, e será dona das leituras de pluviômetro e das vistorias. A tela mostra o veredito sem depender do agente, e o agente o lê pronto, como já faz com o relevo.
2. **O `spray-planner`.** Poria o agente no caminho de uma tela de cadastro e duplicaria a leitura das medições, que são dados da API.
3. **O front.** Viola "regra de negócio não fica em componente" e faria tela e agente discordarem.

### Onde fica na tela

1. **Segunda visão da aba de relevo, renomeada para "Terreno"** (escolhida): `Relevo | Chuva e solo`. As duas respondem "qual máquina entra neste talhão"; relevo é estático e chuva/solo é dinâmico, por isso ficam em visões separadas.
2. **Aba própria.** Separaria a tabela de métodos, que precisa cruzar os dois.

### Validação no front

1. **Tipos TypeScript à mão e serviço em memória que segue o `openapi.yaml`** (escolhida), como o resto do front (ADR 0010). Nenhuma dependência nova.
2. **Zod e MSW**, citados no plano de ação. Seriam duas dependências novas para um único recurso, fora do padrão atual.

## Decisão

1. **Estados do solo.** `SoilState` tem quatro estados ordenados do melhor ao pior para o tráfego (`dry`, `ideal`, `moist`, `waterlogged`) e um quinto, `unknown`, quando não há nenhuma fonte de chuva. `dry` e `ideal` liberam todo o tráfego terrestre; `moist` libera só máquinas leves; `waterlogged` bloqueia o terrestre.
2. **Limites por textura são configuração, marcados como heurística.** `SoilTexture` é `sandy`, `medium` ou `clay`. Para cada textura há limites superiores de chuva acumulada em 48 h (seco, ideal, úmido), as faixas de umidade do modelo como fração da capacidade de campo, o fator das 72 h, a taxa de secagem diária e a data de vigência. Os valores iniciais vêm da tabela do doc de cruzamento e são `[Heurística]`: nada é constante no código. Na fase 1 os padrões ficam em `domain.DefaultSoilTrafficabilityRules` (como `DefaultTerrainRules`); na fase 2 passam para a tabela `trafficability_thresholds`, com `source = heuristic` até a calibração com o agrônomo.
3. **Fronteiras das faixas.** Chuva abaixo do limite de seco é `dry`; até o limite de ideal (inclusive) é `ideal`; até o limite de úmido (inclusive) é `moist`; acima é `waterlogged`. O mesmo vale para a umidade relativa à capacidade de campo (abaixo de 60 % seco, até 90 % ideal, até 100 % úmido).
4. **Regras de combinação**, aplicadas pelo `SoilStateCalculator` nesta ordem:
   1. Vistoria registrada por uma pessoa e ainda válida (menos de 12 h, configurável) substitui o cálculo.
   2. Chuva **medida** (pluviômetro ou estação da fazenda) decide sozinha: a umidade do modelo não entra.
   3. Sem medida, vale o **pior** entre o estado pela chuva do modelo e o estado pela umidade do modelo (o que existir).
   4. Sobre o estado pela chuva: se a chuva de 72 h passar de 1,5 vez (configurável) o limite superior da faixa em que caiu a chuva de 48 h, o estado sobe um degrau (solo que já vinha molhado demora mais a secar). O doc de cruzamento não dizia qual limite; esta é a interpretação adotada.
   5. Sem chuva medida nem do modelo, o estado é `unknown`.
5. **Projeção e liberação.** A partir da chuva de 48 h, cada dia da previsão desconta a taxa de secagem da textura e soma a chuva prevista; o estado do dia sai da mesma tabela. A liberação de um método é o primeiro dia em que o estado projetado atende à exigência dele (`ideal` para pesados, `moist` para leves). Se isso não acontecer dentro do horizonte pedido, não há liberação ("sem liberação prevista"). A projeção é diária e usa só a chuva; a umidade do modelo e a vistoria entram apenas no dia de hoje.
6. **Elegibilidade do método por dia** (`MethodEligibility`), com veredito `apt`, `attention` ou `unfit` e motivos em código:
   - Relevo (ADR 0021): `compatible` não restringe; `restricted` é atenção (`slope_restricted`); `unsuitable` é inapto (`slope_unsuitable`). Relevo ainda não calculado deixa os métodos limitados por declividade em atenção (`terrain_unknown`).
   - Solo: drone e avião ignoram o solo. Pesados (barra, autopropelido, turbopulverizador): `moist` é atenção (`soil_moist`) e `waterlogged` é inapto (`soil_waterlogged`). Costal: `waterlogged` é atenção (`soil_waterlogged`), por conforto e segurança do operador. `unknown` deixa todo método terrestre em atenção (`soil_unknown`), nunca apto: **o estado desconhecido alerta, não bloqueia**.
   - Combinação: vale o pior dos dois, com uma exceção da matriz do doc: relevo restrito com solo úmido ou pior torna o pesado inapto (máquina pesada em encosta molhada escorrega), com o motivo `slope_restricted_on_wet_soil`.
   - O número da fração de área acima do limite vai num campo próprio (`area_above_limit_pct`), não embutido no código do motivo.
7. **Métodos avaliados.** Enquanto não houver cadastro de equipamentos da fazenda, a elegibilidade avalia os seis `SprayMethod` do relevo. O filtro "só métodos cadastrados" entra com esse cadastro.
8. **Contrato.** `GET /farms/{farmId}/fields/{fieldId}/trafficability?days=5`, tag `Trafficability`, `x-status: planned` até a fase 2. Segue o caminho aninhado em fazenda das demais rotas de talhão (o plano citava `/fields/{id}`). Cada número exibido carrega a fonte (`gauge`, `station`, `model`) e o horário.
9. **Leituras e vistorias** são escritas por pessoas, pela tela (`POST .../rain-readings` e `POST .../soil-inspections`, fase 2). O agente só lê o veredito e nunca lança leitura nem vistoria.
10. **Front.** A aba `?tab=terrain` da fazenda passa a se chamar "Terreno" e ganha o seletor `Relevo | Chuva e solo` (`?view=rain`). O front só apresenta o veredito; nenhum cálculo de estado no cliente. Enquanto a API não implementa a rota, a visão usa o serviço em memória com a fixture do Talhão 4.

## Consequências

**Positivas**
- Tela e agente veem exatamente o mesmo veredito, testado sem LLM.
- O agente pode descartar métodos antes de procurar janelas de clima, e nunca recomendar uma janela ótima para um trator que vai atolar.
- Limites como configuração permitem calibrar por região sem mudar código.

**Negativas / custos**
- Limites heurísticos podem bloquear sem necessidade ou deixar passar um atolamento. Por isso o `unknown` só alerta e a origem `heuristic` aparece na resposta.
- A API passará a chamar a Open-Meteo (fase 2) para chuva passada e umidade do solo, além do cliente horário do `spray-planner`. São perguntas diferentes, e cada serviço é dono do seu dado, mas são dois clientes do mesmo provedor.
- A projeção diária só com chuva é grosseira: ignora evapotranspiração e drenagem do talhão. É aceita como primeira versão.

## Pendências

1. ~~Bloqueio desde o início ou só alerta~~ Decidido na fase 2: bloqueia desde o início (adendo abaixo).
2. ~~Vínculo talhão ↔ pluviômetro~~ Decidido na fase 2: vínculo explícito (adendo abaixo).
3. Histórico de estados calculados (`soil_state_history`) para comparar com atolamentos reais (fase 4).
4. Cadastro de equipamentos da fazenda, com limite do fabricante e exigência de solo por máquina.
5. Lacunas entre o frame 04f e o contrato, deixadas fora da tela para não derivar dado no front: o card "Previsão 5 dias" (total e pico do período; propor `forecast_total_mm` e `forecast_peak` na resposta), a linha "Tráfego terrestre: Bloqueado" (propor um campo `ground_traffic` vindo do domínio), a tabela só com métodos cadastrados (depende da pendência 4) e a classe de relevo por método na tabela. Os rótulos de veredito do Figma ("Indicado", "Possível · barro", "Inapto · sem tráfego") diferem dos usados ("Apto", "Atenção", "Inapto"): decidir qual vale.

## Adendo 2026-11-26 (fase 2: dados reais na API)

Decisões aprovadas pelo usuário para a fase 2 (só o back; as telas de lançamento vêm depois):

1. **Bloqueio desde o início.** O veredito sai como o domínio calcula; a resposta traz `thresholds_source = heuristic` para a tela avisar que os limites não foram calibrados. Não há chave para rebaixar "inapto" a "atenção".
2. **Pluviômetro é da fazenda; o vínculo é explícito.** `rain_gauges` pertence a uma fazenda. `rain_gauge_fields` liga cada talhão a no máximo um pluviômetro da mesma fazenda (chave primária no talhão). Talhão sem vínculo usa o modelo.
3. **Leitura pertence ao pluviômetro, não ao talhão.** `POST /farms/{farmId}/rain-gauges/{gaugeId}/readings` substitui o `POST /fields/{id}/rain-readings` do plano: um pluviômetro atende vários talhões, e a leitura vale para todos. Cada leitura é a chuva acumulada desde a anterior (`rain_mm`, `observed_at`); as somas de 24, 48 e 72 h e 7 dias somam as leituras com `observed_at` na janela. Um pluviômetro só vale como fonte se tiver leitura há menos de `API_SOIL_GAUGE_MAX_STALENESS` (padrão 36 h); senão o talhão cai no modelo, porque sem leitura recente não dá para distinguir "não choveu" de "ninguém leu".
4. **Vistoria** em `POST /farms/{farmId}/fields/{fieldId}/soil-inspections` (estado, horário, observação opcional), válida por `API_SOIL_INSPECTION_VALIDITY` (12 h).
5. **Nova capacidade `field_conditions.record`** para `super_admin`, `admin` e `planning_operator`: lançar leitura e vistoria. Cadastrar pluviômetro e definir textura e vínculo do talhão continuam em `registry.write`. A capacidade entra no `openapi.yaml` (`Permission`), no domínio e em `infra/keycloak/agentes-agro.json`; aplicar no Keycloak exige `make keycloak`.
6. **Perfil de solo do talhão** em `GET/PUT /farms/{farmId}/fields/{fieldId}/soil-profile` (`soil_texture`, `rain_gauge_id`), sem mexer no contrato de `Field`. A textura vira coluna `fields.soil_texture` (nula até alguém informar; sem textura o estado é `unknown` com `no_soil_texture`).
7. **Limites em tabela de referência global** `trafficability_thresholds` (sem `company_id`, como `crops`): uma linha por textura e data de vigência, com os limites de chuva e umidade, fator das 72 h, taxa de secagem, **capacidade de campo de referência** (m³/m³; a Open-Meteo não a fornece) e `source` (`heuristic` ou `calibrated`). Semeada com os valores heurísticos; vale a linha mais recente com vigência até hoje.
8. **Open-Meteo na API.** Porta `WeatherProvider` no `usecase`; adaptador HTTP com `past_days=7`, chuva horária e diária, `soil_moisture_0_to_1cm` a `9_to_27cm` (média ponderada pela espessura) e fuso `America/Sao_Paulo`. A coordenada enviada é o centroide do talhão **arredondado a 0,01°** (cerca de 1 km, menor que a célula do modelo): reduz o que sai da empresa para um terceiro e serve de chave de cache. Cache em memória por célula e dia, com validade `API_WEATHER_CACHE_TTL` (1 h), montado na raiz de composição. Falha da Open-Meteo não inventa valor: com pluviômetro, o estado sai da medida e a projeção some (`weather_unavailable`); sem pluviômetro, o estado é `unknown`. Testes com respostas gravadas, nunca a rede.
9. **Auditoria.** Cadastro de pluviômetro, perfil de solo, leitura e vistoria vão para `audit_events`.

## Estado da implementação (2026-11-26)

- **Fase 0:** contrato da rota e dos schemas no `openapi.yaml` (`planned`).
- **Fase 1:** domínio Go (`soil.go`, `soil_rules.go`, `soil_state.go`, `trafficability.go`) com testes de tabela para os casos 1 a 6, 8 e 10 do doc de cruzamento; front com tipos, serviço em memória, componentes e a visão "Chuva e solo" na aba Terreno.
- **Fase 2 (back):** migrações `000022` (índice único `(company_id, farm_id, id)` em `fields`, `CONCURRENTLY`, alvo das FKs que prendem vínculo e vistoria à fazenda do talhão) e `000023` (`fields.soil_texture`, `rain_gauges`, `rain_gauge_fields`, `rain_readings`, `soil_inspections` e `trafficability_thresholds` semeada como heurística, com a capacidade de campo de referência: arenoso 0,15, médio 0,25, argiloso 0,36 m³/m³); casos de uso com auditoria e escopo de empresa e fazenda; adaptador `openmeteo` (centroide arredondado a 0,01°, `past_days=7`, umidade de 0 a 27 cm ponderada pela espessura, fuso `America/Sao_Paulo`, cache em memória por célula e dia); rotas `GET/POST /farms/{farmId}/rain-gauges`, `PATCH .../rain-gauges/{gaugeId}`, `GET/POST .../rain-gauges/{gaugeId}/readings` (intervalo de até 31 dias), `POST .../fields/{fieldId}/soil-inspections`, `GET/PUT .../fields/{fieldId}/soil-profile` e `getFieldTrafficability` como `implemented`; capacidade `field_conditions.record` no domínio, no contrato e em `infra/keycloak/agentes-agro.json` (aplicar com `make keycloak`). O front deixou de usar o mock de trafegabilidade com `NUXT_PUBLIC_API_MOCK_PENDING=true`; as telas de lançamento continuam fora.
- **Decisões da implementação além do adendo:**
  1. Vistoria válida vale mesmo em talhão sem textura (estado de hoje = vistoria, com `no_soil_texture` e projeção `unknown`): quem viu o solo diz como ele está, e ignorar a vistoria deixaria passar um "encharcado" como atenção.
  2. `rain_daily` usa para os dias passados a mesma fonte do estado (pluviômetro com leitura recente ou modelo); dias do modelo saem como `measured` com `source = model`.
  3. Sem a Open-Meteo, `days` traz só hoje e as liberações são calculadas só com hoje.
  4. Leituras e vistorias não se editam nem apagam (o `api_app` não tem `UPDATE`/`DELETE` nelas); corrigir uma leitura exige uma rota nova.
  5. `PUT .../soil-profile` exige as duas chaves (`null` limpa), para um cliente não apagar o vínculo por omissão.
- **Fora deste recorte:** telas de lançamento (pluviômetro, leitura, vistoria, perfil de solo); agente (fase 3, depende de `FindSprayWindows`); operação (fase 4).
