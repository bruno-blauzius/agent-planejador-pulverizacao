# ADR 0021: Relevo do talhão calculado a partir de modelo digital de elevação

- **Status:** aceito e implementado (2026-11-24)
- **Data:** 2026-11-23
- **Telas:** Figma `MeWDfQIHok0s80GbqCKGV3`, seção "Relevo dos talhões · proposta (2026-11-23)" da página `1:4` (frames 04b a 04e, aprovados pelo usuário em 2026-11-23)

## Contexto

A escolha do método de aplicação depende do relevo. A declividade limita trator, autopropelido e turbopulverizador, que correm risco de tombar, perdem tração e fazem a barra oscilar. Drone, avião e costal quase não sofrem esse limite (`docs/dominio/condicoes-pulverizacao.md`, seções 6.7, 6.9 e 7.2). A baixada também concentra ar frio e forma inversão térmica mais cedo.

Hoje o talhão só tem polígono, área e cultura. O doc de domínio diz que declividade, posição no relevo e exposição são **atributos estáticos do talhão, calculados uma vez** a partir de um modelo digital de elevação (MDE), mas não define a fonte, onde se calcula nem onde se guarda.

As telas aprovadas pedem, por talhão:

- declividade média, percentil 90 e máxima;
- classe de relevo da Embrapa e área em cada classe;
- altitude mínima e máxima;
- posição (topo, encosta ou baixada) e exposição;
- um perfil de altitude;
- compatibilidade com cada método de aplicação.

Pedem também um ajuste manual, com justificativa, que não apague o valor calculado.

## Opções

### Fonte da elevação

1. **Copernicus DEM GLO-30** (escolhida). Resolução de 30 m, cobertura global, licença gratuita com atribuição. Distribuído como GeoTIFF otimizado para nuvem (COG), em blocos de 1° × 1° num bucket público (AWS Open Data). Um talhão de 40 ha cobre cerca de 450 pixels, o bastante para uma distribuição de declividade.
2. **SRTM 30 m.** Também tem 30 m, mas é mais antigo (2000) e tem mais falhas em relevo acidentado. Fica como alternativa se o Copernicus ficar indisponível.
3. **API de elevação da Open-Meteo (90 m)**, citada no doc de domínio. Com 90 m, um talhão de 40 ha cobre uns 50 pixels, grosso demais para declividade máxima e percentil. Exigiria uma chamada por ponto e enviaria coordenadas de propriedades a terceiros.
4. **Informado pelo usuário.** Não escala e não tem distribuição. Entra só como ajuste (ver a decisão 5).

### Onde calcular

1. **No Postgres, com a extensão `postgis_raster`** (escolhida). Os blocos do MDE ficam numa tabela de referência e o cálculo é `ST_Clip`, `ST_Slope`, `ST_Aspect` e estatísticas sobre o polígono do talhão, a mesma abordagem do PostGIS para área (ADR 0014) e município (ADR 0019). `ST_FromGDALRaster` monta o raster a partir dos bytes do GeoTIFF, então a API em Go não precisa de GDAL nem de cgo.
2. **Em Go puro.** Exigiria ler COG/GeoTIFF com compressão e reprojetar sem biblioteca madura, ou trazer o GDAL por cgo, o que complica imagem, build e segurança.
3. **Num serviço Python novo** (rasterio). Seria mais um serviço e mais um dono de dado para um cálculo feito uma vez por talhão.

### Quem decide a compatibilidade com o método

1. **O domínio da API** (escolhida). A API é dona do talhão, do relevo e, no futuro, do cadastro de equipamentos. A tela mostra a compatibilidade sem depender de um agente no ar, e o `spray-planner` lê o resultado em vez de recalcular.
2. **A `SlopeRule` do `spray-planner`**, exposta pela API. Isso põe o agente no caminho de uma tela de cadastro e duplica a leitura do relevo.

## Decisão

1. **Fonte.** O MDE é o Copernicus GLO-30. Os blocos são baixados sob demanda, só os que cobrem talhões cadastrados, e guardados numa tabela de referência global `elevation_tiles`, sem `company_id`, como `crops` e `municipalities`. Cada bloco guarda fonte, identificador e versão. O cálculo nunca inventa valor: bloco indisponível marca o talhão como `failed`, com o motivo.
2. **Extensão nova.** `postgis_raster` no banco `api`, criada em `infra/docker/postgres/init/` com o driver GTiff habilitado (`postgis.gdal_enabled_drivers`). Em volume antigo, crie-a manualmente como superusuário, como foi feito com `postgis` e `unaccent`.
3. **O que se guarda**, na tabela `field_terrain` com `company_id NOT NULL`, um registro por talhão:
   - situação (`pending`, `calculating`, `ready`, `failed`) e motivo da falha;
   - fonte, versão do MDE e data do cálculo;
   - declividade média, percentil 90 e máxima (%), e altitude mínima e máxima (m);
   - posição e exposição;
   - área por classe de declividade;
   - o perfil de altitude ao longo da linha de maior declive.

   A **distribuição** é o dado principal, não a média. A classe é derivada da média no domínio da API, nunca gravada como regra no banco.
4. **Parâmetros de negócio em configuração.** Os limites das classes da Embrapa, os limites de declividade de referência por método, o raio usado para classificar a posição e a fração da área acima do limite que torna o método "restrito" ou "inadequado" ficam em `platform`, com valores padrão documentados. Nada disso é número fixo no código.
5. **Ajuste manual.** Fica em `field_terrain_adjustments`, com origem (`topographic_survey` ou `field_assessment`), declividade média e máxima, posição, justificativa obrigatória, autor e data. O valor informado prevalece sobre o calculado, e os dois são devolvidos para comparação. Um novo cálculo automático não apaga o ajuste. Se a média calculada divergir do ajuste acima de um limite configurável, a resposta traz `adjustment_outdated = true`. Remover o ajuste volta ao valor calculado. O ajuste e a remoção vão para `audit_events`.
6. **Quando calcular.** Ao pedido do usuário (`POST /farms/{farmId}/terrain/calculations`) e quando o polígono do talhão muda: o caso de uso de edição marca o relevo como `pending`. O cálculo roda fora da requisição, em segundo plano na própria API, e a tela consulta o estado (`GET /farms/{farmId}/terrain`). Se a API reiniciar no meio, o talhão fica `pending` e entra no próximo pedido. Fila durável só se isso virar problema.
7. **Compatibilidade com o método** (`method_fit`). É calculada no domínio da API: a fração da área acima do limite do método define `compatible`, `restricted` ou `unsuitable`. Métodos sem limite de declividade (costal, drone, avião) são `compatible` para o relevo. Os outros critérios (bula, solo, porte, zona tampão) continuam no `spray-planner`. Quando houver cadastro de equipamentos, o limite do fabricante substitui o de referência. A `SlopeRule` do `spray-planner` passa a consumir esse resultado.
8. **Permissões.** Ler não exige capacidade, como no restante do cadastro. Calcular, ajustar e remover ajuste exigem `registry.write`. Não há permissão nova nem mudança no Keycloak; a nota do Figma que sugeria `terrain.manage` fica superada.
9. **Contrato.** As rotas e os schemas entram no `openapi.yaml` como `planned` (ADR 0010), e o front pode usar o mock até a API implementá-las.

## Consequências

**Positivas**
- O relevo vira dado do talhão, calculado igual para todos e testável sem LLM. O agente recebe a compatibilidade pronta e não faz contas no prompt.
- Guardar a distribuição evita o erro de aprovar um método pela média quando parte do talhão passa do limite.
- O ajuste manual fica auditável e não destrói o dado de origem.

**Negativas / custos**
- Extensão nova (`postgis_raster`) e GDAL dentro do Postgres. A imagem atual já traz o PostGIS completo, mas é preciso confirmar o driver GTiff habilitado e criar a extensão em volumes antigos.
- Os blocos do MDE ocupam espaço: cerca de 25 MB comprimidos por grau quadrado. Uma fazenda costuma cair em 1 ou 2 blocos.
- O download sai para a internet, fora do caminho da requisição. Os testes usam um bloco gravado em `testdata`, nunca a rede.
- O cálculo em segundo plano dentro do processo não é durável. É aceito por enquanto (ver a decisão 6).
- Resolução de 30 m não enxerga terraço nem curva de nível. O ajuste manual existe para isso.

## Pendências

1. Confirmar na imagem `infra/docker/postgres/Dockerfile` que `postgis_raster` e o driver GTiff estão disponíveis.
2. Definir os valores padrão de referência por método (o doc de domínio diz que o limite é do equipamento, não uma constante) e registrá-los no doc de domínio com a fonte.
3. Atribuição da licença do Copernicus na tela "Como calculamos".

## Adendo 2026-11-24 (infra e migrações 000019 a 000021)

Este adendo refina a decisão 1 (tabelas de elevação) e a opção "Onde calcular". Onde divergir do texto acima, vale o adendo.

- **O Postgres só recorta; o Go calcula.** O cálculo de declividade, percentil, posição e exposição é do domínio Go, a partir de janelas de altitude lidas com `ST_Clip` + `ST_DumpValues`. Nenhuma função `ST_Slope`/`ST_Aspect` nem regra de classe mora em SQL (regra de negócio não fica no banco). Isso supera a frase "o cálculo é `ST_Clip`, `ST_Slope`, `ST_Aspect`" da opção escolhida.
- **Blocos como raster em ladrilhos de 256x256.** A API baixa o GeoTIFF de 1° x 1°, o ingere com `ST_FromGDALRaster` + `ST_Tile(256, 256)` e guarda em `elevation_blocks` (índice GiST em `st_convexhull(rast)`). A proveniência fica em `elevation_sources` (`tile_key`, fonte, versão, data, número de blocos); as duas tabelas substituem a `elevation_tiles` prevista. `api_app` só insere e lê; a ingestão grava fonte e blocos na mesma transação.
- **Distribuição guardada para derivar a classe.** `field_terrain.slope_histogram_ha` tem 101 posições (faixas de 1 % de [0,1) a [99,100) e a 101a, "≥ 100 %"), em hectares. A classe de relevo e a fração da área acima do limite do método são derivadas na leitura a partir dela e da configuração, então mudar os limites não exige recalcular os talhões. Registros com `status = 'ready'` exigem todas as medidas (`CHECK`).
- **Execução em `terrain_runs`.** Cada pedido de cálculo (`scope` `missing` ou `all`) vira uma linha com autor e horários; `field_terrain.run_id` aponta para ela, para acompanhar o andamento e reprocessar o que falhou.
- **Ajuste manual** em `field_terrain_adjustments`, um por talhão, removível de verdade (`api_app` mantém `DELETE`). `terrain_runs.requested_by` e `adjusted_by` são FKs simples para `users`, como em `audit_events`, porque o autor pode ser da operadora. `audit_events` não restringe `action`, então as ações `terrain.calculation_requested`, `terrain.adjusted` e `terrain.adjustment_removed` não pedem migração.
- **Pendência 1 resolvida.** A imagem atual (`postgresql-17-postgis-3`, PGDG) já traz `postgis_raster` (versão 3.7.0 na imagem de hoje) com GDAL; o único ajuste é habilitar o driver (`postgis.gdal_enabled_drivers = 'GTiff'`, `ALTER DATABASE` no init) e criar a extensão. A `000019` verifica a extensão e testa o driver de fato.

## Estado da implementação (2026-11-24)

Implementado de ponta a ponta:

- **Banco:** migrações `000019` a `000021` (`postgis_raster`, `elevation_sources`, `elevation_blocks`, `terrain_runs`, `field_terrain`, `field_terrain_adjustments`).
- **API Go:** domínio (classe de relevo, compatibilidade de método, ajuste manual), casos de uso, executor do cálculo, cliente do Copernicus DEM, adaptador Postgres e as 5 operações HTTP da tag `Terrain`.
- **Front:** aba `?tab=terrain` da fazenda, com serviço HTTP real. Em `NUXT_PUBLIC_API_MOCK_PENDING=true` o relevo já usa a API real; o mock (`terrainMock.ts`) serve só o modo `NUXT_PUBLIC_API_MOCK=true`. A conformidade do contrato foi conferida campo a campo entre os DTOs da API e os do front, sem divergência de forma.

Pendências abertas:

- ~~Bloco real não medido~~ Medido em 2026-11-24 (ver "Medição com o bloco real" abaixo). Pendente: medir em produção (rede e memória reais) e com um talhão rural de referência conhecida.
- Os limites de referência de declividade por método precisam de validação com um agrônomo.
- O executor roda dentro do processo, sem fila durável; várias instâncias não se atrapalham (`FOR UPDATE SKIP LOCKED` e lease por idade), mas a recuperação de uma instância caída só ocorre quando o lease expira (ver o adendo da revisão).
- Slope fora de `numeric(7,3)` é tratado como falha do talhão (`failed`), não como erro da execução inteira.

## Medição com o bloco real (2026-11-24)

Teste opt-in `services/api/internal/adapter/postgres/real_tile_integration_test.go` (build tags `integration realtile` e `REAL_TILE=1`; não roda em `go test` nem em `scripts/integration-test.sh`). Comando exato, da pasta `services/api`, com Docker e a imagem `agentes-agro/postgres:dev`:

```bash
bash scripts/real-tile-test.sh
```

O script sobe o Postgres descartável (a mesma biblioteca do `integration-test.sh`, papel `api_app`), aplica as migrações e roda `go test -count=1 -tags 'integration realtile' -run TestRealCopernicusTile -v -timeout 45m ./internal/adapter/postgres/...`. Usa o cliente `copernicus.NewClient` com a configuração padrão (URL real), o `TerrainWorker` com os prazos padrão e o caso de uso `CalculateFarmTerrain`. Cada execução baixa ~42 MB do bucket público (um GET, sem dado do projeto). Rodou duas vezes (a primeira falhou só numa consulta de comparação do próprio teste, já corrigida); os números abaixo são das duas.

**Medido** (bloco `S22W047`, 41 807 083 bytes = 39,87 MiB, GeoTIFF 3600 x 3600 float32; talhão de 41,03 ha no centro de Guaxupé-MG, lat -21,30 lon -46,71):

| Medida | Execução 1 | Execução 2 |
|---|---|---|
| Download (cabeçalhos / corpo completo) | 0,9 s / 30,9 s (1,29 MiB/s) | 0,8 s / 5,4 s (7,38 MiB/s) |
| Ingestão no banco (após o último byte: inspeção, decodificação, `ST_Tile`, inserts) | 5,30 s | 5,06 s |
| `ensureTile` inteiro (HasTile até o fim da ingestão) | 36,2 s | 10,5 s |
| Do `CalculateFarmTerrain` até `ready` | 36,3 s | 10,6 s |
| Pico do heap Go (HeapAlloc) no download / na ingestão | 64 MiB / 161 MiB | 64 MiB / 161 MiB |
| Pico de `Sys` do processo Go | 203 MiB | 211 MiB |
| Pico do contêiner Postgres (`docker stats`, linha de base 62 MiB) | 332 MiB | 608 MiB |

O download depende só da rede (variou 6 vezes entre as duas execuções); a ingestão é estável (~5 s). A memória do Go é 4 vezes o arquivo: `io.ReadAll` cresce o corpo e a ingestão o copia como parâmetro `bytea`. O pico do contêiner vem de apenas 2 a 3 amostras de `docker stats` (que leva ~1,5 s cada) dentro da janela de 5 s, inclui cache de página e variou entre as execuções: leia como ordem de grandeza (algumas centenas de MiB, até ~600 MiB), não como pico exato.

- **Disco:** 225 blocos (não ~2 700: 3600 / 256 = 15, e 15 x 15 = 225), `elevation_sources.block_count` = 225, `pg_total_relation_size('elevation_blocks')` = 51,44 MiB (quase tudo TOAST; heap 0,02 MiB, índices 0,04 MiB), média de 224,1 KiB por bloco. O raster em `bytea` praticamente não comprime (soma 49,23 MiB contra 49,44 MiB de float32 cru): cerca de 1,3 vez o tamanho do GeoTIFF baixado, por bloco de 1 grau.
- **Consulta `Window`:** 5 a 9 ms (10 execuções; grade 57 x 54 = 3 078 células, 483 dentro do talhão). `EXPLAIN (ANALYZE, BUFFERS)`: o planejador usa `ix_elevation_blocks_hull` (GiST) e lê 2 dos 225 blocos, execução de 2,6 ms, 92 buffers, com ou sem `enable_seqscan = off`. A tabela ainda não tinha estatísticas (`last_autoanalyze` nulo, estimativa de 1 linha) e mesmo assim o índice foi escolhido. Com 225 linhas isso não prova a escala (inferido: com milhares de blocos o GiST é o que mantém a leitura em poucos blocos). O cálculo em Go custa 45 microssegundos por chamada.
- **Segunda execução:** outro talhão no mesmo bloco ficou `ready` em 205 ms e o recálculo (`scope=all`) do primeiro em 125 ms; o bloco foi requisitado à origem 1 vez só (0 downloads novos).
- **Falha real:** `FetchTile(S30W030)` (oceano) devolve `ErrElevationUnavailable` (HTTP 404, 0,7 a 0,9 s, sem retentativa); um talhão no oceano fica `failed` com o motivo "bloco do modelo de elevação indisponível" em 0,4 s.

**Resultado do talhão real** (lido de volta do banco; a classe é derivada na leitura, não é gravada): declividade média 9,50 %, p90 16,93 %, máxima 31,75 %; classe `undulating` (limites 3, 8, 20, 45 %); posição `lowland`; exposição `northeast`; altitude 822,5 a 855,0 m; histograma de 42,74 ha (polígono de 41,03 ha, +4 % pela regra do centro da célula) com 5,66 ha `flat`, 14,07 `gently_undulating`, 20,71 `undulating`, 2,30 `strongly_undulating`, 0 `mountainous`; perfil de 24 pontos (842 m no início, mínimo de ~828 m aos 450 m, 838 m no fim, 841 m de comprimento). Aptidão com os limites padrão: `tractor_boom` (limite 20 %) compatível (5,4 % da área acima), `self_propelled` (15 %) restrito (19,3 % acima), `airblast` (20 %) compatível; `knapsack`, `aircraft` e `drone` sem limite de declividade. Plausibilidade: Guaxupé fica a ~820 m e o relevo é suave a ondulado. Ressalva: o ponto escolhido é o centro urbano, e o Copernicus GLO-30 é modelo de **superfície** (inclui edificações e copa), então a declividade aí tem ruído; serve de teste técnico, não de referência agronômica. Um segundo talhão no mesmo bloco (lat -21,62 lon -46,55) deu média 15,5 %, altitude 886 a 965 m.

**Validação independente da declividade (PostGIS).** Mesma janela (talhão mais 100 m), reprojetada para UTM 23S SIRGAS 2000 (EPSG:31983) em grade de 30 m, `ST_Slope(rast, 1, '32BF', 'PERCENT')` e estatística das células com centro dentro do talhão (PostGIS 3.7.0):

| Talhão | Go (média / p90) | `ST_Slope` vizinho mais próximo 30 m | `ST_Slope` bilinear 30 m | bilinear 28,8 m |
|---|---|---|---|---|
| Guaxupé | 9,50 % / 16,93 % | 9,49 % / 17,44 % (+0,1 % / -2,9 %) | 8,91 % / 16,16 % (+6,6 % / +4,7 %) | 9,03 % / 16,51 % (+5,1 % / +2,5 %) |
| Segundo | 15,53 % / 22,28 % | 15,38 % / 22,15 % (+1,0 % / +0,6 %) | 15,02 % / 21,68 % (+3,4 % / +2,8 %) | 15,29 % / 22,14 % (+1,6 % / +0,6 %) |

(diferença relativa do Go sobre o PostGIS.) Sem sinal de erro de escala, sinal ou conversão graus para metros: a média do Go coincide com a do PostGIS sem suavização (0,1 % a 1,0 %). A reamostragem bilinear suaviza o relevo e reduz a declividade (e o máximo, de 31,7 para 27,7 %), por isso o Go fica 2 a 7 % acima dela; o Go lê a grade nativa (~28,8 x 30,9 m a esta latitude), sem reamostrar. As altitudes mínima e máxima também coincidem (822,5 e 855,0 m).

**Prazos e limites com base nos números.** Todos adequados com larga folga; nenhum precisa subir:

- `TileTimeout` (10 min): download + ingestão levaram 10 a 36 s (16 a 60 vezes de folga). Suficiente mesmo com enlace de 0,1 MiB/s (~7 min); manter.
- `API_ELEVATION_REQUEST_TIMEOUT` (2 min por tentativa): o download mais lento levou 31 s. Cobre até ~0,33 MiB/s; em rede mais lenta o corpo estoura e o talhão vira "tempo limite". Se a produção tiver enlace pior, subir para 4 min (a regra `TileTimeout >= timeout x (1 + tentativas)` exigiria então `TileTimeout` de 12 min).
- `FieldTimeout` (2 min): consulta de caixa, `Window` e cálculo somam ~20 ms. Poderia cair para 30 s sem risco; manter 2 min só custa mais tempo de lease.
- Lease (14 min = 10 + 2 + 2): coerente; o trabalho real é de ~40 s, então 14 min só atrasa a recuperação depois de uma queda. Para recuperar mais rápido, `TileTimeout` 5 min, `FieldTimeout` 30 s e folga 1 min dão um lease de 6,5 min.
- `API_ELEVATION_MAX_BYTES` (120 MiB): o bloco real tem 39,9 MiB e o raster cru de um bloco tem ~52 MiB; 120 MiB tem folga de 3 vezes e ainda barra corpo anormal. Manter.
- Memória: reserve pelo menos 512 MiB de limite para o processo da API durante a ingestão (pico de 161 MiB de heap, 211 MiB de `Sys`) e centenas de MiB livres no Postgres (pico observado de 332 a 608 MiB no contêiner). Se isso incomodar, a evolução é ingerir por streaming (o corpo é lido inteiro em memória hoje) ou mover a decodificação para fora do banco (ver "Alternativa descartada").

**Inferido (não medido):** o comportamento em produção com rede e limites de memória reais; o desempenho do GiST com milhares de blocos de vários graus; a ocorrência de timeout com enlace abaixo de 0,33 MiB/s.

## Adendo 2026-11-24 (revisão do executor e da ingestão)

Decisões que a revisão independente do backend pediu e que valem sobre o texto acima.

**Executor: lease por idade, em vez de recuperação global.**
- Cada talhão reivindicado grava `updated_at`. O *lease* é `TileTimeout + FieldTimeout + LeaseSlack` (padrão 10 min + 2 min + 2 min), todos configuráveis (`API_TERRAIN_WORKER_TILE_TIMEOUT`, `API_TERRAIN_WORKER_FIELD_TIMEOUT`, `API_TERRAIN_WORKER_LEASE_SLACK`).
- No `Start` e a cada varredura periódica (`API_TERRAIN_WORKER_SWEEP_INTERVAL`), as linhas `calculating` com `updated_at` mais velho que o lease voltam a `pending` (a `version` sobe, então o save atrasado de quem perdeu o lease vira conflito descartado) e as execuções ociosas são fechadas. Linha mais nova pode ser trabalho de outra instância viva e nunca é tocada. Isso substitui o requeue global do `Start`.
- Consequência: **depois de um crash a recuperação acontece quando o lease expira**, não no instante em que a API sobe. Com os padrões são até 14 minutos. O mesmo vale quando o banco cai justamente ao gravar o resultado e a falha: a linha fica `calculating` até o lease vencer e então é recalculada.
- Um talhão que cobre mais de um bloco recebe `TileTimeout` por bloco, então só nesse caso raro pode passar do lease; o custo é um save descartado e um recálculo, nunca um resultado errado.
- Um pânico em qualquer ponto do processamento (adaptador ou *store*) é registrado e o talhão vira `failed`; não derruba o processo.

**Prazos separados.** `TileTimeout` (padrão 10 min) cobre baixar e ingerir UM bloco; `FieldTimeout` (padrão 2 min) cobre só a consulta de caixa, o recorte da janela e o cálculo. A configuração exige `TileTimeout >= API_ELEVATION_REQUEST_TIMEOUT x (1 + API_ELEVATION_RETRIES)`, ou a API não sobe. O `statement_timeout` da transação de ingestão é derivado do prazo restante do contexto (o `TileTimeout`), com teto de 10 min e piso de 1 s, então os dois nunca discordam.

**Motivos de falha.** "Dados de elevação insuficientes" é o motivo de um talhão sem nenhuma célula com dado dentro do polígono (bloco presente, valores vazios), e de um talhão pequeno demais: menos células com declividade que `API_TERRAIN_MIN_VALID_CELLS` (padrão 20, cerca de 1,8 ha a 30 m). Para esses só vale o ajuste manual. "Bloco indisponível" é cobertura incompleta dos blocos, bloco inexistente ou download cortado no meio (leitura do corpo). Cancelamento do banco por `statement_timeout` (SQLSTATE 57014) e prazo de contexto vencido viram "tempo limite excedido".

**Segurança da origem do GeoTIFF.** `API_ELEVATION_BASE_URL` exige `https` (`http` só para `localhost`, `127.0.0.1` e `::1`, em teste local). O GeoTIFF baixado é interpretado pelo GDAL **dentro do processo do Postgres** (`ST_FromGDALRaster`): é um parser nativo de formato binário vindo de fora. Mitigações: origem confiável por configuração e sobre TLS, driver restrito a GTiff (`postgis.gdal_enabled_drivers`), tamanho máximo do corpo (`API_ELEVATION_MAX_BYTES` e 256 MiB na ingestão) e validação de SRID (EPSG:4326) e de uma única banda antes de gravar qualquer bloco. **Alternativa descartada:** decodificar o GeoTIFF no Go e mandar ao banco só as janelas já lidas; isolaria o parser em um processo sem acesso ao banco, mas exige uma dependência de TIFF/COG e reimplementar o ladrilhamento. Reavaliar se a origem deixar de ser o bucket público do Copernicus.

**Limites conhecidos.**
- A fila é FIFO global entre empresas (a execução mais antiga é atendida primeiro, de qualquer tenant): uma execução grande atrasa as demais. Aceito enquanto o volume for pequeno; justiça por empresa é uma evolução.
- A classe "escarpado" da Embrapa (acima de 75 %) não existe: são 4 limites configuráveis (padrão 3, 8, 20 e 45 %) e "montanhoso" absorve tudo acima de 45 %.
- Aspecto: o vetor médio de descida é normalizado pelo número de células; abaixo de 0,1 de comprimento (cristas e bacias, em que as faces opostas quase se cancelam) o aspecto é `flat`, em vez de apontar um lado por uma diferença mínima.
- `API_TERRAIN_POSITION_RADIUS_M` tem teto de 5000 m.
- `POST /farms/{farmId}/terrain/calculations` responde `202` de forma idempotente: com uma execução aberta devolve o estado atual e não agenda nada novo.

**Otimizações a medir antes de fazer (não feitas agora).** Uma consulta enxuta para `ListFarmTerrain` (hoje traz o histograma e o perfil de todos os talhões) e a dupla decodificação do GeoTIFF na ingestão (`InspectGeoTIFF` e depois `IngestElevationTile`).

**Notas da segunda revisão.**
- O lease compara `updated_at`, gravado com o relógio de quem reivindicou, com o `now` de outra instância. Ele depende, portanto, de relógios razoavelmente sincronizados (NTP); uma defasagem maior que a folga pode devolver à fila um talhão ainda em andamento (custo: save descartado e recálculo).
- O teto de 5000 m de `API_TERRAIN_POSITION_RADIUS_M` existe só em `platform` (carga da configuração), não em `TerrainRules.Validate`.
- O cliente HTTP do Copernicus recusa redirecionamento de https para http e para outro host.
