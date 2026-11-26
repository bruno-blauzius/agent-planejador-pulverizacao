<script setup lang="ts">
import type { StatusTone } from '../StatusBadge.vue'
import type { EligibilityVerdict, TrafficabilityDay } from '~/types/trafficability'
import { methodEligibilityLabel, sprayMethodLabel } from '~/utils/trafficabilityLabels'

/**
 * "Máquina indicada para aplicar hoje" (Figma 84:1468): one row per method with
 * the area above its slope limit, today's verdict (status pill) and, compact,
 * the verdicts of the next days. Verdicts and reasons come from the API
 * (`PlanTrafficability`, ADR 0022); this only presents them.
 */
const props = defineProps<{
  /** Today first, then the projection days. */
  days: readonly TrafficabilityDay[]
}>()

const VERDICT_TONE: Readonly<Record<EligibilityVerdict, StatusTone>> = {
  apt: 'suitable',
  attention: 'attention',
  unfit: 'unsuitable',
}

const percentFormat = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 0 })
const titleId = useId()

const upcomingDays = computed(() => props.days.slice(1))

/** "30/11" from "2026-11-30". */
function shortDay(day: string): string {
  const [, month, date] = day.split('-')
  return month && date ? `${date}/${month}` : day
}

const rows = computed(() => {
  const today = props.days[0]
  if (!today) return []
  return today.methods.map(eligibility => ({
    method: eligibility.method,
    label: sprayMethodLabel(eligibility.method),
    reliefLabel: eligibility.areaAboveLimitPct === null
      ? '—'
      : `${percentFormat.format(eligibility.areaAboveLimitPct)} % acima do limite`,
    tone: VERDICT_TONE[eligibility.verdict],
    verdictLabel: methodEligibilityLabel(eligibility),
    upcoming: upcomingDays.value.map((day) => {
      const verdict = day.methods.find(item => item.method === eligibility.method)
      return {
        date: day.date,
        dayLabel: shortDay(day.date),
        verdict: verdict?.verdict ?? null,
        label: verdict ? methodEligibilityLabel(verdict) : 'sem dado',
      }
    }),
  }))
})
</script>

<template>
  <section class="method-table" :aria-labelledby="titleId">
    <header class="method-table__header">
      <h3 :id="titleId" class="method-table__title text-label-meta-strong">Máquina indicada para aplicar hoje</h3>
      <p class="method-table__hint text-label-tag">Cruza relevo × solo · o agente ainda checa clima e bula</p>
    </header>

    <p v-if="!rows.length" class="method-table__empty text-caption">Sem indicação de método para hoje.</p>

    <!--
      Explicit roles: the phone layout turns rows into blocks (display: block/flex),
      which makes browsers drop the native table semantics.
    -->
    <table v-else class="method-table__table text-caption" role="table" :aria-labelledby="titleId">
      <thead role="rowgroup">
        <tr role="row">
          <th scope="col" role="columnheader">Método</th>
          <th scope="col" role="columnheader">Relevo</th>
          <th scope="col" role="columnheader">Solo hoje → indicação</th>
        </tr>
      </thead>
      <tbody role="rowgroup">
        <tr v-for="row in rows" :key="row.method" role="row">
          <th scope="row" role="rowheader" class="method-table__method">{{ row.label }}</th>
          <td role="cell" class="method-table__relief">
            <span class="method-table__cell-label">Relevo: </span>
            {{ row.reliefLabel }}
          </td>
          <td role="cell" class="method-table__verdict">
            <span class="method-table__cell-label method-table__cell-label--block">Solo hoje → indicação</span>
            <StatusBadge :tone="row.tone" :label="row.verdictLabel" />
            <ul v-if="row.upcoming.length" class="method-table__days" :aria-label="`Próximos dias: ${row.label}`">
              <li
                v-for="day in row.upcoming"
                :key="day.date"
                class="method-table__day text-label-micro"
                :class="day.verdict ? `method-table__day--${day.verdict}` : 'method-table__day--none'"
                :title="`${day.dayLabel}: ${day.label}`"
              >
                <span class="method-table__day-dot" aria-hidden="true" />
                <span aria-hidden="true">{{ day.dayLabel }}</span>
                <span class="visually-hidden">{{ day.dayLabel }}: {{ day.label }}</span>
              </li>
            </ul>
          </td>
        </tr>
      </tbody>
    </table>

    <p class="method-table__note text-caption-xs">
      Limites de solo por textura são configuráveis; uma leitura de pluviômetro substitui o modelo.
    </p>
  </section>
</template>

<style scoped>
.method-table {
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  border: var(--border-width-hairline) solid var(--color-border);
  border-radius: var(--radius-lg);
}

.method-table__header {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4) var(--space-8);
  align-items: baseline;
  padding: var(--space-12) var(--space-14) var(--space-8);
}

.method-table__title {
  flex: 1 1 auto;
  margin: 0;
  color: var(--color-text-primary);
}

.method-table__hint,
.method-table__note,
.method-table__empty {
  margin: 0;
  color: var(--color-text-tertiary);
}

.method-table__empty {
  padding: 0 var(--space-14) var(--space-8);
}

.method-table__table {
  width: 100%;
  border-collapse: collapse;
  text-align: left;
}

.method-table__table th,
.method-table__table td {
  padding: var(--space-8) var(--space-14);
  text-align: left;
  vertical-align: middle;
}

.method-table__table thead th {
  background: var(--color-sand);
  color: var(--color-text-tertiary);
  font-weight: 600;
}

.method-table__table tbody tr {
  border-top: var(--border-width-hairline) solid var(--color-coffee-100);
}

.method-table__method {
  color: var(--color-text-primary);
  font-weight: 700;
}

.method-table__relief {
  color: var(--color-text-secondary);
  white-space: nowrap;
}

.method-table__table .method-table__verdict {
  padding-block: var(--space-6);
}

/* Long verdicts ("Inapto · só na área permitida · solo úmido · …") wrap inside the cell instead of being cut. */
.method-table__verdict :deep(.status-badge) {
  max-width: 100%;
  white-space: normal;
}

.method-table__cell-label {
  display: none;
}

.method-table__days {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-4);
  margin: var(--space-6) 0 0;
  padding: 0;
  list-style: none;
}

.method-table__day {
  --method-day-color: var(--color-text-tertiary);
  --method-day-bg: var(--color-sand);

  display: inline-flex;
  gap: var(--space-4);
  align-items: center;
  padding: var(--space-2) var(--space-6);
  border-radius: var(--radius-pill);
  background: var(--method-day-bg);
  color: var(--method-day-color);
  white-space: nowrap;
}

.method-table__day--apt {
  --method-day-color: var(--color-status-suitable);
  --method-day-bg: var(--color-status-suitable-bg);
}

.method-table__day--attention {
  --method-day-color: var(--color-status-attention);
  --method-day-bg: var(--color-status-attention-bg);
}

.method-table__day--unfit {
  --method-day-color: var(--color-status-unsuitable);
  --method-day-bg: var(--color-status-unsuitable-bg);
}

.method-table__day-dot {
  width: var(--space-6);
  height: var(--space-6);
  border-radius: var(--radius-pill);
  background: currentColor;
}

.method-table__note {
  padding: var(--space-8) var(--space-14) var(--space-12);
}

/* Phones: each method becomes a stacked block, so the table never scrolls sideways. */
@media (max-width: 599px) {
  .method-table__table thead {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }

  .method-table__table,
  .method-table__table tbody,
  .method-table__table tr,
  .method-table__table th,
  .method-table__table td {
    display: block;
  }

  .method-table__table tbody tr {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    padding: var(--space-10) var(--space-14);
  }

  .method-table__table tbody th,
  .method-table__table tbody td,
  .method-table__table .method-table__verdict {
    padding: 0;
  }

  .method-table__cell-label {
    display: inline;
    color: var(--color-text-tertiary);
  }

  .method-table__cell-label--block {
    display: block;
    margin-bottom: var(--space-4);
  }
}
</style>
