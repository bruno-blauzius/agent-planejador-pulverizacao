# ADR 0012: MapLibre GL JS como biblioteca de mapa do front

- **Status:** aceito (2026-11-21)
- **Data:** 2026-11-21

## Contexto

Seis telas do Figma "01 · Acesso e Cadastros" têm mapa: miniaturas e modo mapa da lista de fazendas (7:16), posição da sede e desenho do perímetro no assistente de nova fazenda (14:30, 36:472), importação de perímetro (37:503), mapa da fazenda com talhões e popover (15:56) e desenho de novo talhão (16:82). A página "02 · Planejador de Pulverização" deve trazer ainda mais mapa.

O design do mapa é **próprio**: fundo escuro estilizado (verde café), talhões coloridos por cultura ou por status de aptidão, rótulos em pílula (T1, T2…), seletor "Cultura · Status · Satélite", controles próprios de zoom, localização e desenho (lápis), legenda e popover do talhão. Precisamos de:

1. estilização por dados (cor do polígono conforme cultura/status, destaque do selecionado);
2. desenho e edição de polígonos e de um ponto (sede);
3. exibir GeoJSON vindo da API (perímetros, talhões, perímetro importado);
4. alternar para imagem de satélite;
5. funcionar com o Nuxt em SSR (o mapa só existe no navegador) e respeitar a ADR 0011 (controles do projeto, tokens, nada de visual padrão da biblioteca).

Hoje o `MapView` é estático (SVG exportado do Figma), como combinado na fundação.

## Opções

1. **Leaflet + Leaflet-Geoman.** Simples e maduro; raster por padrão. Estilizar o fundo como no Figma exige tiles raster prontos nesse estilo (não há) ou plugins de vector tiles menos mantidos. Controles padrão a esconder.
2. **MapLibre GL JS + Terra Draw** (escolhida). WebGL, vector tiles com **estilo próprio em JSON** (dá para reproduzir o fundo escuro do Figma), estilização por dados nativa (`match` por cultura/status), boa performance com muitos talhões. Terra Draw desenha/edita polígono e ponto com adaptador oficial para MapLibre. Licenças BSD-3 e MIT.
3. **OpenLayers.** Completo, mas pesado e com API mais verbosa; estilização do fundo igualmente trabalhosa.
4. **Mapbox GL JS.** Equivalente ao MapLibre, porém licença proprietária e token obrigatório.

## Decisão

### Bibliotecas

| Pacote | Tipo | Por quê |
|---|---|---|
| `maplibre-gl` | runtime | renderização do mapa, vector tiles, estilização por dados |
| `terra-draw` + `terra-draw-maplibre-gl-adapter` | runtime | desenho/edição de polígono (perímetro, talhão) e ponto (sede) |
| `@turf/area` | runtime | área **de pré-visualização** enquanto se desenha (ver "Regras") |

Leitura de KML/KMZ/SHP **não** entra no front: o arquivo vai para `POST /perimeter-imports` (ADR 0010), que devolve GeoJSON.

### Fontes de mapa (configuração, não código)

- **Fundo vetorial:** estilo próprio `app/theme/map-style.ts`, gerado a partir de `palette.ts` (fonte única de cores, ADR 0011), aplicado sobre uma fonte de vector tiles configurável por `runtimeConfig.public.mapTilesUrl`. Padrão de desenvolvimento: **OpenFreeMap** (tiles OSM, sem chave).
- **Satélite:** URL de tiles raster configurável por `runtimeConfig.public.mapSatelliteUrl`. O provedor (com termos de uso e eventual chave) é **decisão pendente**; sem URL configurada, a opção "Satélite" fica desabilitada com aviso.
- Atribuição (OSM e provedor de satélite) sempre visível, estilizada com os tokens.

### Arquitetura no front

- A biblioteca vive **apenas** em `app/components/map/` (fachadas da ADR 0011). Telas e composables nunca importam `maplibre-gl` ou `terra-draw`.
- Fachadas previstas:
  - `MapView` passa a ser interativo (mantém `variant: 'thumbnail' | 'farm'` e slot de sobreposição). Recebe GeoJSON por props: `perimeter`, `features` (talhões com `properties` de cultura/status), `colorBy: 'crop' | 'status'`, `selectedId`, `basemap: 'vector' | 'satellite'`; emite `select(id)`, `move(bounds)`. Miniatura = mapa não interativo (sem controles, sem arrastar) ajustado ao perímetro.
  - `MapDrawControl` (dentro do `MapView` ou como slot): modos `polygon` e `point`, `v-model` com GeoJSON; emite `change`.
  - `MapControls`, `MapLegend`, `MapLayerSwitch` e `MapPopover`: componentes Vue próprios (tokens e `AppIcon`), posicionados sobre o mapa. **Os controles padrão do MapLibre não são usados.**
- Carregamento **somente no cliente**: `<ClientOnly>` com import dinâmico de `maplibre-gl`/`terra-draw` dentro da fachada, e placeholder no SSR (o `MapView` estático atual vira o fallback/skeleton). Isso também mantém a biblioteca fora do bundle inicial das páginas sem mapa.
- CSS do `maplibre-gl` importado só na fachada; ajustes de estilo seguem a regra da ADR 0011 (fora de layer, sem `!important`).
- Tipos geográficos: GeoJSON de `app/types/geo.ts` (já existente); as fachadas não expõem tipos da biblioteca.

### Regras

- **Área oficial é da API.** A área exibida enquanto se desenha é uma prévia calculada com `@turf/area` e rotulada como aproximada; ao salvar ou importar, vale a área devolvida pela API. O front não calcula aptidão, Delta T, vento ou chuva — cores por status vêm de dados da API.
- Cores do mapa por cultura/status vêm de tokens/paleta; nenhum hexadecimal novo fora de `palette.ts`.

### Testes

- Lógica de conversão (ex.: features → expressões de estilo, bounds) em funções puras testadas com Vitest.
- O componente de mapa em si é verificado no navegador (WebGL não roda em happy-dom/jsdom); revisão visual contra o Figma, como nas demais fachadas.

## Consequências

**Positivas**
- Fundo, talhões e controles fiéis ao Figma, com estilização por dados e boa performance para muitos talhões (útil também no Planejador).
- Desenho de perímetro, sede e talhão com uma única biblioteca de desenho.
- Bibliotecas isoladas em fachadas e carregadas só no cliente, sem afetar SSR nem as páginas sem mapa.

**Negativas**
- Dependências pesadas (WebGL); navegadores sem WebGL caem no fallback estático.
- Dependência de serviços de tiles externos (OpenFreeMap em dev; provedor de satélite a decidir, com termos e possível custo).
- Mais componentes próprios para manter (controles, legenda, popover).
- Testes automatizados do componente de mapa limitados; a verificação visual é manual.

## Pendências

- Escolher o provedor de imagem de satélite para produção (termos de uso, custo, chave).
- Avaliar hospedar os vector tiles próprios (ex.: PMTiles) se o uso crescer.
