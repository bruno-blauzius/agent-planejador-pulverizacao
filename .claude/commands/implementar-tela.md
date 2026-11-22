---
description: Implementa uma tela do Figma com subagentes de UI e lógica e revisão independente
argument-hint: <link-do-frame-no-figma> [descrição opcional]
---

Implemente a tela do Figma: $ARGUMENTS

Você é o ORQUESTRADOR. Não implemente código diretamente; coordene os subagentes.

1. **Planejamento**: leia o frame no Figma e explore o projeto. Produza um plano curto com: componentes de UI, dados necessários, endpoints da API envolvidos e a lista de arquivos por agente. Mostre o plano e aguarde minha confirmação.
2. **Contrato**: acione o `logic-implementer` para entregar primeiro os tipos e as assinaturas dos composables.
3. **Implementação em paralelo**: com o contrato definido, acione em paralelo:
   - `figma-ui-implementer`, passando o link do Figma e o contrato;
   - `logic-implementer`, para implementar o contrato.
4. **Revisão**: acione o `code-reviewer`, passando o link do Figma, o contrato e os relatórios dos implementadores.
5. **Correções**: se o veredito for REPROVADO, envie cada achado 🔴/🟡 ao agente indicado e repita a revisão. Faça no máximo 2 ciclos; se ainda houver bloqueantes, pare e me mostre o que restou.
6. **Resumo final**: arquivos alterados, veredito do revisor, divergências do design e pendências.
