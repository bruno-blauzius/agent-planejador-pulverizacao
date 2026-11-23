# Contrato — Tela 17: Confirmação humana antes de registrar (Figma 19:8, modal sobre a tela 16)

Lógica: `useApplicationConfirmation` e `utils/planner.ts` (`buildReviewView`). Protocolo: ADR 0016 (proposto).
Regra: **nada é registrado sem a confirmação explícita**, e só um plano da conversa atual, feito para a fazenda selecionada, pode ser revisado. O modal pede uma revisão nova (checagens recalculadas agora) e só chama `POST /applications` com o checkbox "Revisei as condições e confirmo a aplicação" marcado.

## Endpoints (planned)

`POST /planner/plans/{planId}/entries/{fieldId}/review` (não grava) e `POST /applications` (`confirmation.confirmed: true`, `acknowledged_warnings`). Capacidade: `applications.register` (administrador e operador). Erros: 403, 404, 409 `conditions_changed`/`conflict`, 422, 502 `weather_unavailable`.

## `useApplicationConfirmation(options?: { now? })`

```ts
{
  isOpen: Ref<boolean>
  review: ComputedRef<ReviewView | null>
  acknowledged: Ref<boolean>                     // checkbox "Revisei as condições e confirmo a aplicação"
  registered: Ref<RegisteredApplication | null>  // preenchido após o registro (fechar o modal e mostrar o toast)

  canRegister: ComputedRef<boolean>              // can('applications.register')
  canSubmit: ComputedRef<boolean>                // permissão + canConfirm + checkbox + revisão válida + sem envio em curso + ainda não registrado
  isExpired: ComputedRef<boolean>                // a revisão passou de expires_at; o relógio só é lido ao abrir e ao enviar (sem timer), então só muda nesses momentos
  loading: Ref<boolean>; submitting: Ref<boolean>
  loadError: ComputedRef<ApiError | null>; submitError: ComputedRef<ApiError | null>
  isForbidden: ComputedRef<boolean>              // sem capacidade, 403 ou out_of_scope
  isStale: ComputedRef<boolean>                  // revisão expirada ou 409 conditions_changed
  isAlreadyRegistered: ComputedRef<boolean>      // 409 conflict
  isWeatherUnavailable: ComputedRef<boolean>
  isBlocked: ComputedRef<boolean>                // alguma checagem `blocked`
  isPlanOutOfContext: ComputedRef<boolean>       // plano de outra conversa/fazenda ou talhão fora do plano: nada é pedido à API; texto "Gere um novo plano"

  open(planId: string, fieldId: string): Promise<boolean>   // abre e carrega a revisão; false sem permissão ou em erro
  reload(): Promise<boolean>                                // nova revisão da mesma entrada (após isStale); limpa checkbox e erros
  close(): void
  submit(): Promise<RegisteredApplication | null>           // null se !canSubmit ou em erro (ver submitError)
}

ReviewView = {
  id, fieldTitle /* "T3 · Catuaí 144" */, areaLabel /* "38,6 ha" */,
  dayLabel /* "Qua · 26/11" */, timeLabel /* "06:00 – 10:00" */,
  productName /* "Fungicida" */, doseLabel /* "0,5 L/ha · calda 200 L/ha" */,
  mixVolumeLabel /* "7.720 L" */, tanksLabel /* "· 4 tanques" */,
  checks: { code, status: 'ok'|'warning'|'blocked', icon: 'check'|'warning'|'blocked', title, detail }[],   // "Checagens do agente" (6; a 7ª, `window_fits`, aparece só com janela curta)
  operatorEquipment /* "Carlos Mendes · Pulverizador 2.000 L" */,
  canConfirm, warningCodes /* reconhecidos pelo checkbox */, blockedTitles
}
```

## Estados

| Estado | Sinal | Comportamento |
|---|---|---|
| Carregando revisão | `loading` | esqueleto; botão "Confirmar e registrar" desabilitado |
| Pronto | `review` | botões "Voltar e ajustar" (`close`) e "Confirmar e registrar" (`canSubmit`) |
| Aviso | checks com `warning` | linha em tom de atenção; o checkbox reconhece todos |
| Bloqueado | `isBlocked`, `review.blockedTitles` | botão desabilitado e motivo visível; só "Voltar e ajustar" |
| Plano fora do contexto | `isPlanOutOfContext` | "Este plano não é desta fazenda. Gere um novo plano."; trocar de fazenda fecha o modal sozinho |
| Revisão desatualizada | `isStale` | "As condições mudaram. Revise novamente." com botão que chama `reload()` |
| Já registrada | `isAlreadyRegistered` | "Esta aplicação já foi registrada." |
| Erro de carga/envio | `loadError` / `submitError` | mensagem da API + tentar de novo |
| Sem permissão | `isForbidden` | "Você não tem permissão para registrar aplicações"; sem botão de confirmar |
| Sucesso | `registered` | fechar o modal e exibir toast ("Aplicação registrada") |

Não há "Ajustar" estruturado: "Voltar e ajustar" só fecha o modal.
