import { describe, expect, it } from 'vitest'
import { useApplicationConfirmation } from '~/composables/useApplicationConfirmation'
import { usePlannerChat } from '~/composables/usePlannerChat'
import { usePlannerContext } from '~/composables/usePlannerContext'
import { usePlannerWindows } from '~/composables/usePlannerWindows'
import { useTodayTask } from '~/composables/useTodayTask'
import { AUTH_TOKEN_COOKIE } from '~/composables/useAuthToken'
import { useSession } from '~/composables/useSession'
import { flushPromises } from '../../support/nuxtFakes'
import { SCENARIO, signInWithPlannerFaults } from '../../support/planner'
import { SEED_LOGINS, SEED_NOW, signInAs } from '../../support/session'

const now = () => SEED_NOW

async function setupContext(login: string = SEED_LOGINS.admin, withFields = true) {
  await signInAs(login)
  const context = usePlannerContext()
  await flushPromises()
  if (withFields) context.setFields([SCENARIO.fieldT3, SCENARIO.fieldT7])
  return context
}

describe('usePlannerContext', () => {
  it('starts on the user farm with its fields, fungicide and nothing selected', async () => {
    const context = await setupContext(SEED_LOGINS.admin, false)

    expect(context.farmId.value).toBe(SCENARIO.farmId)
    expect(context.productKind.value).toBe('fungicide')
    expect(context.fieldOptions.value.find(option => option.value === SCENARIO.fieldT3)).toMatchObject({ label: 'T3', name: 'Talhão 3', areaHa: 38.6 })
    expect(context.productOptions.map(option => option.label)).toEqual(['Fungicida', 'Inseticida', 'Herbicida', 'Foliar', 'Outro'])
    expect(context.farmOptions.value.map(option => option.value)).toContain(SCENARIO.farmId)
    expect(context.requestContext.value).toBeNull()
    expect(context.isReady.value).toBe(false)
  })

  it('builds the request context from the selected fields and clears them when the farm changes', async () => {
    const context = await setupContext()

    expect(context.selectedFieldsLabel.value).toBe('T3 · T7')
    expect(context.requestContext.value).toEqual({ farmId: SCENARIO.farmId, fieldIds: [SCENARIO.fieldT3, SCENARIO.fieldT7], productKind: 'fungicide' })

    context.toggleField(SCENARIO.fieldT3)
    expect(context.fieldIds.value).toEqual([SCENARIO.fieldT7])
    context.toggleField(SCENARIO.fieldT3)
    expect(context.fieldIds.value).toEqual([SCENARIO.fieldT7, SCENARIO.fieldT3])

    context.setProductKind('herbicide')
    expect(context.requestContext.value?.productKind).toBe('herbicide')

    context.selectFarm(SCENARIO.farmId)
    expect(context.fieldIds.value).toHaveLength(2)
    context.selectFarm(null)
    expect(context.fieldIds.value).toEqual([])
    expect(context.requestContext.value).toBeNull()
  })

  it('operators see only their released farm', async () => {
    const context = await setupContext(SEED_LOGINS.operator, false)
    expect(context.farmOptions.value.map(option => option.value)).toEqual([SCENARIO.farmId])
  })
})

describe('usePlannerChat', () => {
  it('does not call the agent without farm and fields', async () => {
    const context = await setupContext(SEED_LOGINS.admin, false)
    const chat = usePlannerChat({ context, now })

    expect(chat.contextMissing.value).toBe(true)
    expect(await chat.send('Quando faço?')).toBe(false)
    expect(chat.entries.value).toEqual([])
    expect(chat.isEmpty.value).toBe(true)
  })

  it('sends the draft, shows both bubbles, the plan and the suggestions', async () => {
    const context = await setupContext()
    const chat = usePlannerChat({ context, now })
    chat.draft.value = '  Preciso aplicar fungicida nos talhões 3 e 7 essa semana. Quando faço?  '

    expect(await chat.send()).toBe(true)

    expect(chat.draft.value).toBe('')
    expect(chat.entries.value.map(entry => entry.role)).toEqual(['user', 'agent'])
    expect(chat.entries.value[0]?.paragraphs).toEqual(['Preciso aplicar fungicida nos talhões 3 e 7 essa semana. Quando faço?'])
    const agent = chat.entries.value[1]!
    expect(agent.paragraphs).toHaveLength(3)
    expect(agent.consulted).toHaveLength(4)
    expect(chat.planView.value?.headline).toBe('2 janelas aptas cobrem os 90 ha')
    expect(chat.planView.value?.entries.map(entry => entry.detail)).toEqual(['38,6 ha · 3h15', '51,4 ha · 4h20'])
    expect(chat.planView.value?.hasAlerts).toBe(true)
    expect(chat.suggestions.value).toHaveLength(3)
    expect(chat.conversationId.value).toMatch(/^conversation-/)
    expect(chat.pending.value).toBe(false)
    expect(chat.error.value).toBeNull()
  })

  it('keeps the conversation and the last plan across messages', async () => {
    const context = await setupContext()
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    const conversationId = chat.conversationId.value

    await chat.send(chat.suggestions.value[0]!)

    expect(chat.conversationId.value).toBe(conversationId)
    expect(chat.entries.value).toHaveLength(4)
    // The answer about rain has no plan: the card keeps the previous one.
    expect(chat.entries.value[3]?.plan).toBeNull()
    expect(chat.planView.value?.headline).toBe('2 janelas aptas cobrem os 90 ha')
  })

  it('exposes an honest error when the agent is down and retries without duplicating the question', async () => {
    const faults = { agent: true }
    await signInWithPlannerFaults(faults)
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })

    expect(await chat.send('Quando faço?')).toBe(false)
    expect(chat.isAgentUnavailable.value).toBe(true)
    expect(chat.isWeatherUnavailable.value).toBe(false)
    expect(chat.canRetry.value).toBe(true)
    expect(chat.entries.value.map(entry => entry.role)).toEqual(['user'])

    faults.agent = false
    expect(await chat.retry()).toBe(true)
    expect(chat.entries.value.map(entry => entry.role)).toEqual(['user', 'agent'])
    expect(chat.error.value).toBeNull()
    expect(chat.canRetry.value).toBe(false)
  })

  it('flags weather outages and resets', async () => {
    await signInWithPlannerFaults({ weather: true })
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })

    await chat.send('Quando faço?')
    expect(chat.isWeatherUnavailable.value).toBe(true)

    chat.reset()
    expect(chat.entries.value).toEqual([])
    expect(chat.canRetry.value).toBe(false)
  })

  it('clears conversation, plan and suggestions when the farm changes', async () => {
    const context = await setupContext()
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    expect(chat.latestPlan.value).not.toBeNull()
    chat.draft.value = 'rascunho'

    context.selectFarm('farm-boa-esperanca')
    await flushPromises()

    expect(chat.entries.value).toEqual([])
    expect(chat.latestPlan.value).toBeNull()
    expect(chat.planView.value).toBeNull()
    expect(chat.conversationId.value).toBeNull()
    expect(chat.suggestions.value).toEqual([])
    expect(chat.draft.value).toBe('')
  })

  it('drops an answer that arrives after the farm changed', async () => {
    const context = await setupContext()
    const chat = usePlannerChat({ context, now })

    const sending = chat.send('Quando faço?')
    context.selectFarm('farm-boa-esperanca')
    await flushPromises()
    expect(await sending).toBe(false)

    expect(chat.entries.value).toEqual([])
    expect(chat.latestPlan.value).toBeNull()
    expect(chat.canRetry.value).toBe(false)
  })

  it('hides the error of the previous farm after a reset', async () => {
    await signInWithPlannerFaults({ agent: true })
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    expect(chat.isAgentUnavailable.value).toBe(true)

    context.selectFarm('farm-boa-esperanca')
    await flushPromises()

    expect(chat.error.value).toBeNull()
    expect(chat.isAgentUnavailable.value).toBe(false)
  })

  it('clears chat and selection on logout', async () => {
    const fakes = await signInAs(SEED_LOGINS.admin)
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    expect(chat.entries.value).toHaveLength(2)

    useSession().logout()
    await flushPromises()

    expect(fakes.cookie(AUTH_TOKEN_COOKIE).value).toBeNull()
    expect(chat.entries.value).toEqual([])
    expect(chat.latestPlan.value).toBeNull()
    expect(chat.conversationId.value).toBeNull()
    expect(context.fieldIds.value).toEqual([])
    // Nobody is signed in: no farm. A later login follows its own primary farm.
    expect(context.farmId.value).toBeNull()
  })

  it('ignores a blank message', async () => {
    const context = await setupContext()
    const chat = usePlannerChat({ context, now })
    expect(await chat.send('   ')).toBe(false)
    expect(chat.entries.value).toEqual([])
  })
})

describe('usePlannerWindows', () => {
  it('loads the 5-day map and the conditions of the first day', async () => {
    const context = await setupContext()
    const windows = usePlannerWindows({ context })
    await flushPromises()

    expect(windows.map.value.rows.map(row => row.fieldLabel)).toEqual(['T3', 'T7'])
    expect(windows.map.value.days.map(day => day.label)).toEqual(['Sáb 21', 'Dom 22', 'Seg 23', 'Ter 24', 'Qua 25'])
    expect(windows.map.value.updatedLabel).toBe('dados de hoje 05:40')
    expect(windows.selectedDate.value).toBe('2026-11-21')
    expect(windows.conditions.value.title).toBe('Sábado 21/11 · condições hora a hora')
    expect(windows.conditions.value.window?.label).toBe('Janela 06–10h')
    expect(windows.conditions.value.stats.map(stat => stat.value)).toEqual(['2,6 – 6,2 °C', '6 km/h', '0 mm'])
    expect(windows.windows.value.length).toBeGreaterThan(0)
    expect(windows.pending.value).toBe(false)
    expect(windows.error.value).toBeNull()
    expect(windows.isEmpty.value).toBe(false)
  })

  it('switches day and window', async () => {
    const context = await setupContext()
    const windows = usePlannerWindows({ context })
    await flushPromises()

    windows.selectDay('2026-11-22')
    await flushPromises()
    expect(windows.conditions.value.window).toBeNull()
    expect(windows.conditions.value.stats).toEqual([])

    windows.selectWindow('2026-11-21', '07:00', '09:00')
    await flushPromises()
    expect(windows.selectedDate.value).toBe('2026-11-21')
    expect(windows.conditions.value.window?.label).toBe('Janela 07–09h')
  })

  it('is empty while no farm is selected', async () => {
    const context = await setupContext(SEED_LOGINS.admin, false)
    context.selectFarm(null)
    const windows = usePlannerWindows({ context })
    await flushPromises()

    expect(windows.isEmpty.value).toBe(true)
    expect(windows.conditions.value.isEmpty).toBe(true)
  })

  it('reports a weather outage', async () => {
    await signInWithPlannerFaults({ weather: true })
    const context = usePlannerContext()
    await flushPromises()
    const windows = usePlannerWindows({ context })
    await flushPromises()

    expect(windows.isWeatherUnavailable.value).toBe(true)
    expect(windows.error.value?.code).toBe('weather_unavailable')
    expect(windows.isForbidden.value).toBe(false)
  })
})

async function chatWithPlan() {
  const context = await setupContext()
  const chat = usePlannerChat({ context, now })
  await chat.send('Quando faço?')
  return chat.latestPlan.value!
}

describe('useApplicationConfirmation', () => {
  it('opens with the review, requires the checkbox and registers', async () => {
    const plan = await chatWithPlan()
    const confirmation = useApplicationConfirmation({ now })

    expect(await confirmation.open(plan.id, SCENARIO.fieldT3)).toBe(true)
    expect(confirmation.isOpen.value).toBe(true)
    expect(confirmation.review.value).toMatchObject({
      fieldTitle: 'T3 · Catuaí 144',
      areaLabel: '38,6 ha',
      mixVolumeLabel: '7.720 L',
      tanksLabel: '· 4 tanques',
      canConfirm: true,
    })
    expect(confirmation.review.value?.checks).toHaveLength(6)

    expect(confirmation.canSubmit.value).toBe(false)
    expect(await confirmation.submit()).toBeNull()
    expect(confirmation.registered.value).toBeNull()

    confirmation.acknowledged.value = true
    expect(confirmation.canSubmit.value).toBe(true)
    const application = await confirmation.submit()

    expect(application).toMatchObject({ status: 'planned', fieldLabel: 'T3' })
    expect(confirmation.registered.value).toEqual(application)
    expect(confirmation.canSubmit.value).toBe(false)
    expect(confirmation.submitError.value).toBeNull()
  })

  it('acknowledges the warning of a short window by the same checkbox', async () => {
    const plan = await chatWithPlan()
    const confirmation = useApplicationConfirmation({ now })

    await confirmation.open(plan.id, SCENARIO.fieldT7)
    expect(confirmation.review.value?.warningCodes).toEqual(['window_fits'])
    confirmation.acknowledged.value = true
    expect(await confirmation.submit()).toMatchObject({ fieldLabel: 'T7' })
  })

  it('closes and resets', async () => {
    const plan = await chatWithPlan()
    const confirmation = useApplicationConfirmation({ now })
    await confirmation.open(plan.id, SCENARIO.fieldT3)
    confirmation.acknowledged.value = true

    confirmation.close()

    expect(confirmation.isOpen.value).toBe(false)
    expect(confirmation.review.value).toBeNull()
    expect(confirmation.acknowledged.value).toBe(false)
  })

  it('keeps the button off when a check is blocked', async () => {
    const faults = { blockedCheck: 'wind' as const }
    await signInWithPlannerFaults(faults)
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    const confirmation = useApplicationConfirmation({ now })

    await confirmation.open(chat.latestPlan.value!.id, SCENARIO.fieldT3)
    confirmation.acknowledged.value = true

    expect(confirmation.isBlocked.value).toBe(true)
    expect(confirmation.review.value?.blockedTitles).toEqual(['Vento adequado'])
    expect(confirmation.canSubmit.value).toBe(false)
    expect(await confirmation.submit()).toBeNull()
  })

  it('flags stale conditions and recovers after reloading the review', async () => {
    const faults = { conditionsChanged: true }
    await signInWithPlannerFaults(faults)
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    const confirmation = useApplicationConfirmation({ now })
    await confirmation.open(chat.latestPlan.value!.id, SCENARIO.fieldT3)
    confirmation.acknowledged.value = true

    expect(await confirmation.submit()).toBeNull()
    expect(confirmation.isStale.value).toBe(true)
    expect(confirmation.submitError.value?.code).toBe('conditions_changed')

    faults.conditionsChanged = false
    expect(await confirmation.reload()).toBe(true)
    expect(confirmation.isStale.value).toBe(false)
    expect(confirmation.submitError.value).toBeNull()
    expect(confirmation.acknowledged.value).toBe(false)
    confirmation.acknowledged.value = true
    expect(await confirmation.submit()).not.toBeNull()
  })

  it('treats an expired review as stale without calling the API', async () => {
    const plan = await chatWithPlan()
    let clock = SEED_NOW
    const confirmation = useApplicationConfirmation({ now: () => clock })
    await confirmation.open(plan.id, SCENARIO.fieldT3)
    confirmation.acknowledged.value = true
    expect(confirmation.canSubmit.value).toBe(true)

    clock = new Date(SEED_NOW.getTime() + 16 * 60_000)
    expect(await confirmation.submit()).toBeNull()
    expect(confirmation.isExpired.value).toBe(true)
    expect(confirmation.isStale.value).toBe(true)
    expect(confirmation.canSubmit.value).toBe(false)
    expect(confirmation.submitError.value).toBeNull()
    expect(confirmation.registered.value).toBeNull()
  })

  it('reports a second registration as already registered', async () => {
    const plan = await chatWithPlan()
    const first = useApplicationConfirmation({ now })
    await first.open(plan.id, SCENARIO.fieldT3)
    first.acknowledged.value = true
    await first.submit()

    // The entry is already registered in the conversation: no new review, no 409 round trip.
    const second = useApplicationConfirmation({ now })
    expect(await second.open(plan.id, SCENARIO.fieldT3)).toBe(false)
    expect(second.isAlreadyRegistered.value).toBe(true)
    expect(second.review.value).toBeNull()
    second.acknowledged.value = true
    expect(second.canSubmit.value).toBe(false)
    expect(second.registeredFieldIds.value).toEqual([SCENARIO.fieldT3])
  })

  it('remembers registered entries for the latest plan, shows the notice and forgets them with the conversation', async () => {
    const context = await setupContext()
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    const plan = chat.latestPlan.value!
    const confirmation = useApplicationConfirmation({ now })
    expect(confirmation.registeredFieldIds.value).toEqual([])
    expect(confirmation.notice.value).toBeNull()

    await confirmation.open(plan.id, SCENARIO.fieldT3)
    confirmation.acknowledged.value = true
    await confirmation.submit()

    expect(confirmation.notice.value).toEqual({ title: 'Aplicação registrada', message: 'T3 · 21/11 06:00 – 10:00. Ela entra como planejada.' })
    expect(confirmation.registeredFieldIds.value).toEqual([SCENARIO.fieldT3])
    expect(confirmation.isFieldRegistered(plan.id, SCENARIO.fieldT3)).toBe(true)
    expect(confirmation.isFieldRegistered(plan.id, SCENARIO.fieldT7)).toBe(false)
    // T7 is still open for registration.
    expect(await confirmation.open(plan.id, SCENARIO.fieldT7)).toBe(true)

    context.selectFarm('farm-boa-esperanca')
    await flushPromises()
    expect(confirmation.registeredFieldIds.value).toEqual([])
    expect(confirmation.isFieldRegistered(plan.id, SCENARIO.fieldT3)).toBe(false)
  })

  it('has no permission: nothing is requested and the screen is forbidden', async () => {
    const plan = await chatWithPlan()
    const session = useState<{ permissions: string[] } | null>('session:user', () => null)
    session.value = { ...session.value!, permissions: [] }
    const confirmation = useApplicationConfirmation({ now })

    expect(confirmation.canRegister.value).toBe(false)
    expect(await confirmation.open(plan.id, SCENARIO.fieldT3)).toBe(false)
    expect(confirmation.isForbidden.value).toBe(true)
    expect(confirmation.loadError.value).toBeNull()
    expect(confirmation.review.value).toBeNull()
    confirmation.acknowledged.value = true
    expect(confirmation.canSubmit.value).toBe(false)
    expect(await confirmation.submit()).toBeNull()
  })

  it('shows the API 403 on the review and on the registration', async () => {
    const faults = { forbidden: false }
    await signInWithPlannerFaults(faults)
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    const planId = chat.latestPlan.value!.id

    const onSubmit = useApplicationConfirmation({ now })
    await onSubmit.open(planId, SCENARIO.fieldT3)
    onSubmit.acknowledged.value = true
    expect(onSubmit.isForbidden.value).toBe(false)
    faults.forbidden = true
    expect(await onSubmit.submit()).toBeNull()
    expect(onSubmit.isForbidden.value).toBe(true)
    expect(onSubmit.submitError.value?.code).toBe('forbidden')
    expect(onSubmit.registeredFieldIds.value).toEqual([])

    const onReview = useApplicationConfirmation({ now })
    expect(await onReview.open(planId, SCENARIO.fieldT3)).toBe(false)
    expect(onReview.isForbidden.value).toBe(true)
    expect(onReview.loadError.value?.code).toBe('forbidden')
  })

  it('reports a weather outage on the review', async () => {
    const faults = { weather: false }
    await signInWithPlannerFaults(faults)
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    const confirmation = useApplicationConfirmation({ now })

    faults.weather = true
    expect(await confirmation.open(chat.latestPlan.value!.id, SCENARIO.fieldT3)).toBe(false)
    expect(confirmation.isWeatherUnavailable.value).toBe(true)
    expect(confirmation.loadError.value).toMatchObject({ code: 'weather_unavailable', status: 502 })
    expect(confirmation.review.value).toBeNull()
    expect(confirmation.isForbidden.value).toBe(false)
  })

  it('exposes a generic registration failure without marking anything registered', async () => {
    const faults = { registrationFailure: true }
    await signInWithPlannerFaults(faults)
    const context = usePlannerContext()
    await flushPromises()
    context.setFields([SCENARIO.fieldT3])
    const chat = usePlannerChat({ context, now })
    await chat.send('Quando faço?')
    const confirmation = useApplicationConfirmation({ now })
    await confirmation.open(chat.latestPlan.value!.id, SCENARIO.fieldT3)
    confirmation.acknowledged.value = true

    expect(await confirmation.submit()).toBeNull()
    expect(confirmation.submitError.value).toMatchObject({ code: 'internal_error', status: 500 })
    expect(confirmation.isStale.value).toBe(false)
    expect(confirmation.isAlreadyRegistered.value).toBe(false)
    expect(confirmation.isForbidden.value).toBe(false)
    expect(confirmation.registered.value).toBeNull()
    expect(confirmation.notice.value).toBeNull()
    expect(confirmation.registeredFieldIds.value).toEqual([])

    // Recovers on retry.
    faults.registrationFailure = false
    expect(await confirmation.submit()).not.toBeNull()
    expect(confirmation.submitError.value).toBeNull()
  })

  it('refuses a plan that is not from this conversation, without asking the API', async () => {
    await signInAs(SEED_LOGINS.admin)
    const confirmation = useApplicationConfirmation({ now })

    expect(await confirmation.open('plan-unknown', SCENARIO.fieldT3)).toBe(false)
    expect(confirmation.isOpen.value).toBe(true)
    expect(confirmation.isPlanOutOfContext.value).toBe(true)
    expect(confirmation.loadError.value).toBeNull()
    expect(confirmation.review.value).toBeNull()
    confirmation.acknowledged.value = true
    expect(confirmation.canSubmit.value).toBe(false)
    confirmation.close()
    expect(confirmation.isPlanOutOfContext.value).toBe(false)
  })

  it('refuses a field that is not in the plan', async () => {
    const plan = await chatWithPlan()
    const confirmation = useApplicationConfirmation({ now })

    expect(await confirmation.open(plan.id, 'field-str-1')).toBe(false)
    expect(confirmation.isPlanOutOfContext.value).toBe(true)
  })

  it('refuses a plan made for another farm and closes the modal when the farm changes', async () => {
    const plan = await chatWithPlan()
    const confirmation = useApplicationConfirmation({ now })
    await confirmation.open(plan.id, SCENARIO.fieldT3)
    expect(confirmation.review.value).not.toBeNull()

    usePlannerContext().selectFarm('farm-boa-esperanca')
    await flushPromises()
    expect(confirmation.isOpen.value).toBe(false)
    expect(confirmation.review.value).toBeNull()

    expect(await confirmation.open(plan.id, SCENARIO.fieldT3)).toBe(false)
    expect(confirmation.isPlanOutOfContext.value).toBe(true)
  })
})

describe('useTodayTask', () => {
  // Greeting uses the viewer's local hour, so the clock is built in local time.
  const morning = () => new Date(2026, 10, 21, 7, 0)

  async function setupToday(login: string = SEED_LOGINS.operator) {
    await signInAs(login)
    const today = useTodayTask({ now: morning })
    await flushPromises()
    return today
  }

  it('builds the operator screen of the Figma', async () => {
    const today = await setupToday()

    expect(today.greeting.value).toBe('Bom dia, Carlos')
    expect(today.locationLabel.value).toBe('Faz. Santa Rita · Talhão 3')
    expect(today.status.value).toMatchObject({
      headline: 'Pode pulverizar',
      nowLabel: 'AGORA · 06:48',
      untilLabel: 'até 10:00',
      remainingLabel: '· faltam 3h12',
      startLabel: '06:00',
      endLabel: '10:00',
    })
    expect(today.metrics.value.map(metric => [metric.label, metric.valueLabel, metric.isOk])).toEqual([
      ['Delta T', '4,2 °C', true],
      ['Vento', '6 km/h', true],
      ['Chuva 4h', '0 mm', true],
    ])
    expect(today.hours.value.map(hour => hour.label)).toEqual(['06', '07', '08', '09', '10', '11', '12', '13', '14', '15', '16', '17', '18'])
    expect(today.hours.value.filter(hour => hour.isCurrent).map(hour => hour.label)).toEqual(['06'])
    expect(today.hours.value.map(hour => hour.condition).slice(0, 7)).toEqual(['suitable', 'suitable', 'suitable', 'suitable', 'attention', 'attention', 'unsuitable'])
    expect(today.task.value).toEqual({
      id: expect.any(String),
      fieldLabel: 'T3',
      title: 'Fungicida · 38,6 ha',
      detail: '0,5 L/ha · 4 tanques de 2.000 L',
      isStarted: false,
    })
    expect(today.canStart.value).toBe(true)
    expect(today.isEmpty.value).toBe(false)
    expect(today.pending.value).toBe(false)
  })

  it('starts the application and refreshes the task', async () => {
    const today = await setupToday()

    expect(await today.start()).toBe(true)
    await flushPromises()

    expect(today.task.value?.isStarted).toBe(true)
    expect(today.canStart.value).toBe(false)
    expect(today.startError.value).toBeNull()
    expect(await today.start()).toBe(false)
  })

  it('reports a start conflict (409) when the task was already started', async () => {
    await signInAs(SEED_LOGINS.operator)
    const first = useTodayTask({ now: morning })
    const second = useTodayTask({ now: morning })
    await flushPromises()

    expect(await first.start()).toBe(true)
    // `second` still shows the task as not started (loaded before): the API says no.
    expect(second.canStart.value).toBe(true)
    expect(await second.start()).toBe(false)
    await flushPromises()

    expect(second.startConflict.value).toBe(true)
    expect(second.startForbidden.value).toBe(false)
    expect(second.startError.value).toMatchObject({ code: 'conflict', status: 409 })
    // After the refresh it is in progress and can no longer be started.
    expect(second.task.value?.isStarted).toBe(true)
    expect(second.canStart.value).toBe(false)
  })

  it('reports 403 when starting and when loading', async () => {
    const faults = { forbidden: false }
    await signInWithPlannerFaults(faults, SEED_LOGINS.operator)
    const today = useTodayTask({ now: morning })
    await flushPromises()
    expect(today.isForbidden.value).toBe(false)

    faults.forbidden = true
    expect(await today.start()).toBe(false)
    expect(today.startForbidden.value).toBe(true)
    expect(today.startConflict.value).toBe(false)
    expect(today.startError.value?.code).toBe('forbidden')
    expect(today.task.value?.isStarted).toBe(false)

    const blocked = useTodayTask({ now: morning })
    await flushPromises()
    expect(blocked.isForbidden.value).toBe(true)
    expect(blocked.status.value).toBeNull()
    expect(blocked.isEmpty.value).toBe(false)
  })

  it('is empty when there is no task for the user farm, but still shows the conditions', async () => {
    const today = await setupToday('joao@ouroverde.test')

    expect(today.error.value).toBeNull()
    expect(today.isEmpty.value).toBe(true)
    expect(today.task.value).toBeNull()
    expect(today.canStart.value).toBe(false)
    expect(today.status.value?.headline).toBe('Pode pulverizar')
    expect(today.metrics.value).toHaveLength(3)
    expect(today.locationLabel.value).toBe('Faz. Ouro Verde')
    expect(await today.start()).toBe(false)
  })

  it('reports a weather outage instead of inventing conditions', async () => {
    await signInWithPlannerFaults({ weather: true }, SEED_LOGINS.operator)
    const today = useTodayTask({ now: morning })
    await flushPromises()

    expect(today.isWeatherUnavailable.value).toBe(true)
    expect(today.status.value).toBeNull()
    expect(today.task.value).toBeNull()
    expect(today.isEmpty.value).toBe(false)
  })
})
