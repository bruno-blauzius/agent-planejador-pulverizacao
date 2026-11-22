import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFlash } from '~/composables/useFlash'
import { useOwnerForm } from '~/composables/useOwnerForm'
import { ApiError } from '~/services/errors'
import type { ApiServices } from '~/services/ports'
import {
  isValidCnpj,
  isValidCpf,
  maskDateInput,
  maskDocumentInput,
  maskPhoneInput,
  OWNER_FORM_MESSAGES,
  parseBrDate,
  shareAdjustmentHint,
} from '~/utils/ownerForm'
import { createTestApi } from '../../support/mockApi'
import { flushPromises, installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const VALID_CPF = '529.982.247-25'
const VALID_CNPJ = '11.222.333/0001-81'
const today = () => new Date('2026-11-21T12:00:00Z')

function fillIndividual(form: ReturnType<typeof useOwnerForm>, document = VALID_CPF) {
  form.name.value = 'Ana Souza'
  form.document.value = document
  form.birthDate.value = '12031979'
  form.phone.value = '35998120099'
  form.email.value = 'ana@email.test'
}

describe('owner form helpers', () => {
  it('masks document, phone and date while typing', () => {
    expect(maskDocumentInput('5299822', 'individual')).toBe('529.982.2')
    expect(maskDocumentInput('529982247251234', 'individual')).toBe('529.982.247-25')
    expect(maskDocumentInput('11222333000181', 'company')).toBe('11.222.333/0001-81')
    expect(maskPhoneInput('35998124470')).toBe('(35) 99812-4470')
    expect(maskPhoneInput('3532140000')).toBe('(35) 3214-0000')
    expect(maskDateInput('12031979')).toBe('12/03/1979')
  })

  it('validates CPF/CNPJ check digits', () => {
    expect(isValidCpf(VALID_CPF)).toBe(true)
    expect(isValidCpf('529.982.247-24')).toBe(false)
    expect(isValidCpf('111.111.111-11')).toBe(false)
    expect(isValidCnpj(VALID_CNPJ)).toBe(true)
    expect(isValidCnpj('11.222.333/0001-80')).toBe(false)
  })

  it('parses only real dates', () => {
    expect(parseBrDate('12/03/1979')).toBe('1979-03-12')
    expect(parseBrDate('31/02/2020')).toBeNull()
    expect(parseBrDate('12/03')).toBeNull()
  })

  it('hints the share adjustment of the wizard', () => {
    const shares = [{ sharePercent: 60 }, { sharePercent: 40 }]
    expect(shareAdjustmentHint(shares, 20, 'Helena Prado')).toBe('Com 20% para Helena, ajuste os demais para fechar 100%.')
    expect(shareAdjustmentHint([{ sharePercent: 80 }], 20, 'Helena')).toBeNull()
    expect(shareAdjustmentHint(shares, null, 'Helena')).toBeNull()
  })
})

describe('useOwnerForm · create', () => {
  it('blocks the Dados step with pt-BR field errors', () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'create', today })
    form.document.value = '529.982.247-24'
    form.birthDate.value = '01/01/2030'
    form.email.value = 'ana@'

    expect(form.goToLinks()).toBe(false)
    expect(form.step.value).toBe('data')
    expect(form.fieldErrors.value).toEqual({
      name: OWNER_FORM_MESSAGES.nameRequired.individual,
      document: OWNER_FORM_MESSAGES.documentInvalid.individual,
      birthDate: OWNER_FORM_MESSAGES.birthDateFuture,
      email: OWNER_FORM_MESSAGES.emailInvalid,
    })
  })

  it('lets someone without WhatsApp pass Dados and asks for it only at the alerts toggle', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'create', today })
    fillIndividual(form)
    form.phone.value = ''

    expect(form.goToLinks()).toBe(true)
    expect(form.fieldErrors.value.windowAlerts).toBeUndefined()

    expect(await form.submit()).toBeNull()
    expect(form.step.value).toBe('links')
    expect(form.fieldErrors.value).toEqual({ windowAlerts: OWNER_FORM_MESSAGES.phoneRequiredForAlerts })

    form.windowAlertsEnabled.value = false
    expect(form.fieldErrors.value.windowAlerts).toBeUndefined()
    const result = await form.submit()
    expect(result?.owner).toMatchObject({ phone: null, windowAlertsEnabled: false })
  })

  it('shows the share error only after touching the field or submitting', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'create', today })
    fillIndividual(form)
    form.goToLinks()
    await flushPromises()
    const errorOf = (farmId: string) => form.links.value.find(link => link.farmId === farmId)?.error

    form.toggleFarm('farm-santa-rita', true)
    expect(errorOf('farm-santa-rita')).toBeNull()
    form.touchShare('farm-santa-rita')
    expect(errorOf('farm-santa-rita')).toBe(OWNER_FORM_MESSAGES.shareRequired)

    form.toggleFarm('farm-boa-esperanca', true)
    expect(await form.submit()).toBeNull()
    expect(errorOf('farm-boa-esperanca')).toBe(OWNER_FORM_MESSAGES.shareRequired)
  })

  it('switches labels and mask with the person type', () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'create', today })
    form.personType.value = 'company'
    form.document.value = '11222333000181'

    expect(form.labels.value).toEqual({ name: 'Razão social', document: 'CNPJ' })
    expect(form.document.value).toBe(VALID_CNPJ)
    expect(form.documentHint.value).toBe('CNPJ válido')
    form.document.value = '11222333000180'
    expect(form.documentHint.value).toBeNull()
  })

  it('shows farm totals and refuses a total above 100%', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'create', today })
    fillIndividual(form)
    expect(form.goToLinks()).toBe(true)
    await flushPromises()

    form.toggleFarm('farm-santa-rita', true)
    form.setShare('farm-santa-rita', '20')
    const santaRita = form.links.value.find(link => link.farmId === 'farm-santa-rita')

    expect(santaRita).toMatchObject({ othersShare: 100, totalShare: 120, totalShareLabel: '120%', error: OWNER_FORM_MESSAGES.farmTotalOver })
    expect(await form.submit()).toBeNull()
    expect(form.step.value).toBe('links')
  })

  it('creates the owner, saves links and leaves the flash for the list', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'create', today })
    fillIndividual(form)
    form.goToLinks()
    await flushPromises()
    form.toggleFarm('farm-recanto', true)
    form.setRole('farm-recanto', 'tenant')

    const result = await form.submit()

    expect(result).toMatchObject({ mode: 'create', owner: { name: 'Ana Souza', document: '52998224725', birthDate: '1979-03-12', phone: '35998120099' } })
    expect(result?.mode === 'create' && result.links).toEqual([{ farmId: 'farm-recanto', farmName: 'Fazenda Recanto', role: 'tenant', sharePercent: null }])
    expect(useFlash().consume()).toMatchObject({ title: 'Cadastro de Ana Souza salvo', highlightId: result?.owner.id })
  })

  it('puts a duplicated document (409) on the document field', async () => {
    installNuxtFakes(createTestApi().api)
    const first = useOwnerForm({ mode: 'create', today })
    fillIndividual(first)
    await first.submit()

    const second = useOwnerForm({ mode: 'create', today })
    fillIndividual(second)
    expect(await second.submit()).toBeNull()

    expect(second.fieldErrors.value.document).toBe('CPF já cadastrado')
    expect(second.formError.value).toBeNull()
    second.name.value = 'Ana S.'
    await flushPromises()
    expect(second.fieldErrors.value.document).toBeUndefined()
  })

  it('does not create the owner twice when saving links fails', async () => {
    let failLinks = true
    let api: ApiServices | null = null
    installNuxtFakes(() => {
      const base = createTestApi().api
      api = {
        ...base,
        owners: {
          ...base.owners,
          replaceLinks: async (ownerId, links) => {
            if (failLinks) throw new ApiError({ code: 'network_error', message: 'Sem conexão.', status: 0 })
            return base.owners.replaceLinks(ownerId, links)
          },
        },
      }
      return api
    })
    const form = useOwnerForm({ mode: 'create', today })
    fillIndividual(form)

    expect(await form.submit()).toBeNull()
    expect(form.formError.value).toBe('Sem conexão.')

    failLinks = false
    const result = await form.submit()
    expect(result?.mode).toBe('create')
    expect((await api!.owners.list({ search: 'Ana Souza' })).total).toBe(1)
  })
})

describe('useOwnerForm · edit', () => {
  it('loads the owner with the document locked and the pending-document notice', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'edit', ownerId: 'owner-helena', today })
    await flushPromises()

    expect(form.loading.value).toBe(false)
    expect(form.name.value).toBe('Helena Prado')
    expect(form.isDocumentLocked.value).toBe(true)
    expect(form.document.value).toBe('***.330.884-**')
    form.document.value = '000'
    expect(form.document.value).toBe('***.330.884-**')
    expect(form.phone.value).toBe('(35) 99812-4470')
    expect(form.pendingDocumentNotice.value).toEqual({
      title: 'Documento com foto pendente',
      description: 'Sem ele, Helena não pode assinar o registro de aplicações.',
    })
    expect(form.linksSummary.value).toBe('Vínculos: Sítio Boa Esperança · 50%')
    expect(form.links.value.find(link => link.farmId === 'farm-boa-esperanca')).toMatchObject({ selected: true, othersShare: 50, totalShare: 100 })
  })

  it('updates data keeping the original document and links', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'edit', ownerId: 'owner-helena', today })
    await flushPromises()
    form.phone.value = '35998120000'

    const result = await form.submit()

    expect(result).toMatchObject({ mode: 'edit', owner: { document: '71533088420', phone: '35998120000' } })
    expect(result?.mode === 'edit' && result.links.map(link => link.farmId)).toEqual(['farm-boa-esperanca'])
    expect(useFlash().consume()?.title).toBe('Dados de Helena Prado atualizados')
  })
})

describe('useOwnerForm · wizard', () => {
  it('creates only the owner and returns the share, without flash or links', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({
      mode: 'wizard',
      today,
      presetFarm: { name: 'Fazenda Santa Rita', currentShares: [{ name: 'Bruno Blauzius', sharePercent: 60 }, { name: 'Marina Rezende', sharePercent: 40 }] },
    })
    fillIndividual(form)
    form.wizardShare.value = '20'

    expect(form.wizardShareContext.value).toEqual({
      currentLabel: 'Hoje: Bruno 60% · Marina 40%',
      hint: 'Com 20% para Ana, ajuste os demais para fechar 100%.',
    })
    const result = await form.submit()

    expect(result).toMatchObject({ mode: 'wizard', owner: { name: 'Ana Souza', windowAlertsEnabled: true }, sharePercent: 20 })
    expect(result?.owner.id && (await useOwnerFormLinks(result.owner.id))).toEqual([])
    expect(useFlash().consume()).toBeNull()
  })

  it('requires the share in the wizard', async () => {
    installNuxtFakes(createTestApi().api)
    const form = useOwnerForm({ mode: 'wizard', today, presetFarm: { name: 'F', currentShares: [] } })
    fillIndividual(form)

    expect(await form.submit()).toBeNull()
    expect(form.fieldErrors.value.wizardShare).toBe(OWNER_FORM_MESSAGES.shareRequired)
  })
})

/** Links of an owner straight from the services wired by the fakes. */
function useOwnerFormLinks(ownerId: string) {
  return useNuxtApp().$api.owners.getLinks(ownerId)
}
