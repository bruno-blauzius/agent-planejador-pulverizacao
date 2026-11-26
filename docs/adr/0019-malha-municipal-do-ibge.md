# ADR 0019: Malha municipal do IBGE para identificar o município do perímetro

- **Status:** aceito
- **Data:** 2026-11-23

## Contexto

`POST /perimeter-imports` devolve `municipality` e `state` para preencher o cadastro da fazenda, mas a API não tinha como descobri-los: os campos saíam sempre nulos. O IBGE publica nomes e limites dos cerca de 5.570 municípios numa API pública de dados (`servicodados.ibge.gov.br`).

## Opções

1. **Malha do IBGE no banco** (escolhida). Uma tabela de referência com os limites e uma consulta de ponto em polígono com PostGIS.
2. **Geocodificação reversa externa** (ex.: Nominatim) a cada importação. Põe rede externa no caminho da requisição, tem limite de uso e envia coordenadas de propriedades a terceiros.
3. **Manter nulo**, deixando o usuário digitar.

## Decisão

- **Tabela `municipalities`** (migração `000017`): código IBGE com 7 dígitos, nome, UF e `boundary geometry(MultiPolygon, 4326)` com índice GiST. É referência global como `crops`, sem `company_id`, e o `api_app` só lê. A tabela nasce vazia.
- **Carga:** o comando `cmd/load-municipalities` (`make api-load-municipalities`) baixa os nomes (`/api/v1/localidades/municipios?view=nivelado`) e a malha (`/api/v3/malhas/paises/BR?intrarregiao=municipio&formato=application/vnd.geo+json`), junta os dois pelo código e substitui a tabela numa transação.
  - A qualidade da malha é configurável em `API_IBGE_MESH_QUALITY`: `minima`, `intermediaria` (padrão) ou `maxima`.
  - O comando grava com o papel `api_migrator`, como o seed de culturas, porque é dado de referência e não dado de runtime.
  - `ST_MakeValid` corrige auto-interseções que a generalização da malha produz.
  - As duas APIs do IBGE são atualizadas em momentos diferentes. Um município criado depois da última malha tem nome mas não tem limite: em 2026-11, o 5101837, Boa Esperança do Norte (MT). Um município extinto pode manter limite sem nome. Esses casos ficam de fora e aparecem no log como aviso.
  - A carga recusa resposta parcial: código repetido ou menos de `API_IBGE_MIN_MUNICIPALITIES` municípios (5.500 por padrão).
- **Uso:** a importação mede o perímetro, pega um ponto garantidamente interno (`ST_PointOnSurface`, porque o centróide de um perímetro côncavo pode cair fora) e procura o município que o contém (`ST_Covers`). Na divisa, vence o menor código. Sem malha carregada ou fora do Brasil, a resposta é nula, nunca inventada.

## Consequências

**Positivas**
- Município e UF são preenchidos sem serviço externo no caminho da requisição.

**Negativas / custos**
- A carga baixa algumas dezenas de MB da internet (qualidade intermediária) e precisa ser repetida quando o IBGE atualizar a malha (anual). É manual por enquanto.
- A malha generalizada pode errar o município em perímetros colados na divisa. O usuário revisa o valor no assistente.
- Os testes usam respostas gravadas (`internal/adapter/ibge/testdata`), nunca a API real. O teste de integração grava com o `migrator` e lê com o `api_app`.
