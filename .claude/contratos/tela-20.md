# Contrato — Tela 20: Aplicação · detalhe (painel lateral de 520 px; Figma 63:627 e painel 63:890)

Abre sobre a tela 19 (`/applications?application=<id>`, via `useApplicationListPage().openDetail(id)`). Lógica: `useApplicationDetail` e `buildApplicationDetailView` (`utils/applications.ts`). Todos os números (dose total, calda total, tanques, carência, veredito) chegam calculados pela API.

## Endpoint (planned, ADR 0016)

`GET /applications/{applicationId}`. Erros: 401, 403 `out_of_scope`, 404 (inclui aplicação de outra empresa).

## `useApplicationDetail(applicationId: MaybeRefOrGetter<string | null | undefined>)`

```ts
{
  detail: ComputedRef<ApplicationDetail | null>
  view: ComputedRef<ApplicationDetailView | null>
  pending: ComputedRef<boolean>; error: ComputedRef<ApiError | null>
  isForbidden: ComputedRef<boolean>         // forbidden ou out_of_scope
  isNotFound: ComputedRef<boolean>          // 404: "Esta aplicação não existe mais"
  refresh(): Promise<void>

  canRepeat: ComputedRef<boolean>           // carregada e can('planner.use')
  planRepeat(): string | null               // preenche fazenda, talhão e produto do Planejador e devolve '/planner' (null sem permissão ou antes de carregar); navegue com o retorno
  downloadAction: { disabled: true, hint: 'Em breve' }   // "Baixar registro (PDF)": sem endpoint
}
// Com id null o painel está fechado e nada é pedido.

ApplicationDetailView = {
  status: { value, label /* "Concluída" */, tone },
  registeredLabel /* "Registrada em 21/11 às 09:52" */,
  title /* "Inseticida · Talhão 5" */,
  subtitle /* "Sáb 21/11 · 06:12 – 09:40 · 56,3 ha · Catuaí 144" */,
  conditions: { headline /* "Dentro da janela" */,
                stats: { key: 'delta_t'|'wind'|'rain'|'duration', label, value }[] /* Delta T 4,2 °C; Vento 6 km/h; Chuva 8h 0 mm; Duração 3h28 */ } | null,   // card escuro "Condições registradas na aplicação"
  productRows: { label, value }[],   // "Produto e calda": Produto "Inseticida · lote 2409-B" · Dose "0,3 L/ha · 16,9 L no total" · Calda "180 L/ha · 10.134 L · 6 tanques" · Carência "14 dias · colheita liberada a partir de 05/12"
  teamRows: { label, value }[],      // "Equipe e equipamento": Operador · Pulverizador · Confirmado por "Marina Rezende · 20/11 às 17:40"
  signatureLabel: string | null,     // "Registro assinado digitalmente · válido para o caderno de campo" (faixa verde); null sem assinatura
  cancellationLabel: string | null   // motivo, quando cancelada
}
```

Linhas ausentes não aparecem: sem lote, "Produto" mostra só o nome; sem `pre_harvest`, não há "Carência"; sem `released_from` (planejada), mostra só "14 dias"; sem confirmação, não há "Confirmado por".

## Estados

| Estado | Sinal | Comportamento |
|---|---|---|
| Carregando | `pending` | painel em esqueleto, botão fechar ativo |
| Pronto | `view` | cabeçalho com selo de status, card de condições, cartões de produto e equipe, faixa de assinatura |
| Planejada | `view.conditions === null` | sem card escuro de condições (a previsão ao vivo fica na tela 16); sem faixa de assinatura |
| Cancelada | `view.cancellationLabel` | mostra o motivo no lugar do card de condições; "Planejar repetição" segue disponível |
| 404 | `isNotFound` | "Esta aplicação não existe mais" + fechar (`closeDetail`) |
| 403 / fora do escopo | `isForbidden` | "Você não tem acesso a esta aplicação" + fechar |
| Erro | `error` | mensagem da API + "Tentar de novo" (`refresh`) |

## Ações

- **Baixar registro (PDF):** desabilitado "Em breve" (`downloadAction`).
- **Planejar repetição:** `const path = detail.planRepeat(); if (path) navigateTo(path)`. Não registra nada: o agente propõe e o usuário confirma na tela 17. Esconder/desabilitar sem `canRepeat`.
- **Fechar:** `closeDetail()` da lista (remove `?application=`).
