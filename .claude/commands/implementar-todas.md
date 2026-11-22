---
description: Implementa todas as telas de um arquivo/página do Figma, em etapas, com revisão independente por tela
argument-hint: <link-do-arquivo-ou-página-no-figma> [instruções opcionais]
---

Implemente todas as telas do Figma: $ARGUMENTS

Você é o ORQUESTRADOR. Não implemente código diretamente; coordene os subagentes `figma-ui-implementer`, `logic-implementer` e `code-reviewer`. Mantenha seu próprio contexto enxuto: guarde apenas resumos e vereditos dos subagentes, nunca o código completo.

## Arquivo de progresso
Use `.claude/progresso-telas.md` como fonte da verdade do andamento.
- Se o arquivo já existir, NÃO recomece: leia-o e continue da primeira tela que não esteja `✅ aprovada`.
- Atualize-o ao fim de cada etapa, para que a execução possa ser retomada se for interrompida.

Formato:
```
| # | Tela | node-id | Depende de | Status | Veredito | Observações |
```
Status possíveis: `⏳ pendente`, `🔧 em andamento`, `✅ aprovada`, `⚠️ bloqueada`.

## Etapa 1 — Inventário
1. Leia a estrutura do arquivo/página no Figma (`get_metadata`) e liste os frames de nível superior que representam telas. Ignore rascunhos, frames de documentação, variantes de componentes e frames duplicados (ex.: mesma tela em mobile e desktop — agrupe como uma tela responsiva).
2. Para cada tela, registre nome, node-id e uma descrição de uma linha.
3. Identifique **componentes compartilhados**: elementos que aparecem em 2 ou mais telas (header, sidebar, cards, tabelas, formulários, modais, estados vazios).
4. Identifique **dados compartilhados**: entidades e endpoints usados por mais de uma tela.
5. Ordene as telas por dependência (telas base primeiro, ex.: layout/listagem antes de detalhe/edição).
6. Crie o arquivo de progresso e me mostre o inventário, os compartilhados e a ordem. **Aguarde minha confirmação** antes de continuar. Eu posso remover, reordenar ou agrupar telas.

## Etapa 2 — Fundação (uma única vez)
1. Acione o `logic-implementer` para criar os tipos e composables dos dados compartilhados.
2. Acione o `figma-ui-implementer` para criar os componentes compartilhados e o layout base, verificando antes o que já existe no projeto.
3. Acione o `code-reviewer` sobre a fundação. Corrija até no máximo 2 ciclos.
4. Registre no progresso a lista dos componentes e composables compartilhados criados. Essa lista deve ser passada a todos os subagentes nas etapas seguintes, com a instrução de reaproveitar e não recriar.

## Etapa 3 — Telas, uma por vez
Para cada tela pendente, na ordem do inventário:
1. Marque como `🔧 em andamento`.
2. Contrato: `logic-implementer` entrega tipos e assinaturas específicos da tela.
3. Em paralelo: `figma-ui-implementer` (com node-id da tela, contrato e lista de compartilhados) e `logic-implementer` (implementação do contrato).
4. `code-reviewer` revisa apenas o diff desta tela, comparando com o screenshot do Figma.
5. Se REPROVADO, envie os achados 🔴/🟡 ao agente indicado e revise de novo, no máximo 2 ciclos.
6. Atualize o progresso:
   - aprovada → `✅ aprovada` com o veredito;
   - ainda com bloqueantes → `⚠️ bloqueada`, com o resumo do problema, e siga para a próxima tela que não dependa dela.
7. Se a tela precisar alterar um componente compartilhado, faça a alteração de forma compatível e peça ao revisor para verificar as telas já aprovadas que o usam.

Telas são processadas **uma por vez** para evitar conflitos em arquivos compartilhados. Só processe duas telas em paralelo se eu pedir explicitamente e se elas não tocarem nos mesmos arquivos.

## Pausas
- A cada 3 telas aprovadas, mostre um resumo curto e pergunte se devo continuar.
- Pare imediatamente e me consulte se: o design for ambíguo, faltar endpoint na API, ou uma mudança exigir alterar mais de 3 telas já aprovadas.

## Resumo final
Tabela final do progresso, telas bloqueadas e motivo, divergências de design acumuladas e pendências de API.
