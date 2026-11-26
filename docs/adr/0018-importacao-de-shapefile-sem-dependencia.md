# ADR 0018: Importação de Shapefile com leitor próprio

- **Status:** aceito
- **Data:** 2026-11-23

## Contexto

`POST /perimeter-imports` lia KML, KMZ e GeoJSON, e respondia 415 para Shapefile (`.zip`). O Shapefile é o formato do CAR (Cadastro Ambiental Rural), de onde vêm muitos perímetros de fazenda. Ler o formato exige decodificar `.shp`, que é binário, e saber o sistema de coordenadas pelo `.prj`. Reprojetar de UTM para WGS84 exige PROJ, uma biblioteca em C que precisa de cgo.

## Opções

1. **Leitor próprio com a biblioteca padrão** (escolhida). O formato de polígonos é pequeno e estável (especificação ESRI de 1998). Aceita só coordenadas geográficas.
2. **Biblioteca `go-shp`.** Dependência externa pouco mantida, que tem a mesma limitação de sistema de coordenadas.
3. **GDAL ou PROJ via cgo.** Reprojeta qualquer sistema, mas traz cgo, imagem maior e build mais difícil.
4. **Manter o 415.**

## Decisão

- O pacote `internal/adapter/perimeter` (`shapefile.go`) lê o `.zip`. Ele precisa ter **um** `.shp` e o `.prj` de mesmo nome; o `.dbf` não é necessário, porque só a geometria interessa. Tipos aceitos: Polygon, PolygonZ e PolygonM, com Z e M ignorados.
- Os anéis seguem a regra do formato: anel externo em sentido horário e furo em anti-horário. Cada furo vai para o anel externo cujo retângulo envolvente contém o primeiro vértice dele. Se o arquivo ignorar a orientação, todo anel é tratado como externo. Cada registro é uma geometria, e as regras de extração são as mesmas dos outros formatos: o maior polígono é o perímetro e os demais contam em `field_count`.
- **Sistema de coordenadas:** aceita GEOGCS WGS 84 ou SIRGAS 2000, que é o datum do CAR e coincide com WGS84 na escala de uma fazenda. A resposta informa o datum em `datum`. Coordenada projetada (PROJCS, como UTM), outro datum (SAD69, Córrego Alegre) ou `.prj` ausente respondem 422 com a orientação de exportar em coordenadas geográficas.
- Limites contra arquivos maliciosos: `.shp` descompactado até 64 MB, `.prj` até 64 KB, até 10.000 registros. Contagens e deslocamentos são conferidos contra o tamanho real do registro.
- `.shp` avulso, sem `.zip`, responde 422 pedindo o `.zip`.

## Consequências

**Positivas**
- Arquivos do CAR importam sem dependência nova e sem cgo.

**Negativas / custos**
- Arquivo em UTM precisa ser reexportado em coordenadas geográficas pelo usuário (QGIS, ArcGIS). A mensagem de erro diz como.
- Polígonos com furos dentro de outros furos (ilhas) podem ter o furo associado ao anel errado quando os retângulos envolventes se sobrepõem. Isso é raro em perímetro de fazenda, e o banco ainda valida a geometria (`ST_IsValid`).
- Se a reprojeção virar requisito, a evolução é a opção 3, com novo ADR.
