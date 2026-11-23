import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useApiServices } from '~/composables/useApiServices'
import { useCompanyForm } from '~/composables/useCompanyForm'
import type { Company } from '~/types/admin'
import { flushPromises } from '../../support/nuxtFakes'
import { SEED_LOGINS, signInAs } from '../../support/session'

afterEach(() => vi.unstubAllGlobals())

const VALID_CNPJ = '11.222.333/0001-81'

function fill(form: ReturnType<typeof useCompanyForm>, document = VALID_CNPJ) {
  form.name.value = '  Fazenda Nova '
  form.document.value = document
  form.city.value = 'Alfenas'
  form.state.value = 'mg'
}

describe('useCompanyForm (create)', () => {
  it('starts empty on the basic plan and shows errors only after touching', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useCompanyForm({ mode: 'create' })
    expect(form.title).toBe('Nova empresa')
    expect(form.plan.value).toBe('basic')
    expect(form.fieldErrors.value).toEqual({})

    form.touch('name')
    expect(form.fieldErrors.value).toEqual({ name: 'Informe o nome da empresa.' })
  })

  it('does not call the API while invalid and touches every field', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useCompanyForm({ mode: 'create' })
    expect(await form.submit()).toBeNull()
    expect(Object.keys(form.fieldErrors.value).sort()).toEqual(['city', 'document', 'name', 'state'])
  })

  it('creates an active company on a paid plan', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useCompanyForm({ mode: 'create' })
    fill(form)
    form.plan.value = 'pro'

    const result = await form.submit()
    expect(result).toMatchObject({
      mode: 'create',
      company: { name: 'Fazenda Nova', document: '11222333000181', state: 'MG', plan: 'pro', status: 'active' },
    })
    expect(form.formError.value).toBeNull()
  })

  it('creates a company in evaluation on the trial plan', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const form = useCompanyForm({ mode: 'create' })
    fill(form)
    form.plan.value = 'trial'
    expect(form.planHint.value).toMatch(/avaliação/)

    const result = await form.submit()
    expect(result?.company).toMatchObject({ plan: 'trial', status: 'trial' })
    expect(result?.company.trialEndsAt).toBeInstanceOf(Date)
  })

  it('puts a duplicated CNPJ on the document field and clears it when the user edits', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const first = useCompanyForm({ mode: 'create' })
    fill(first)
    expect(await first.submit()).not.toBeNull()

    const form = useCompanyForm({ mode: 'create' })
    fill(form)
    expect(await form.submit()).toBeNull()
    expect(form.fieldErrors.value.document).toBe('Já existe uma empresa com este CNPJ.')
    expect(form.formError.value).toBeNull()
    expect(form.saving.value).toBe(false)

    form.document.value = '11.444.777/0001-61'
    await flushPromises()
    expect(form.fieldErrors.value.document).toBeUndefined()
  })

  it('shows a 403 in the footer for a non super admin', async () => {
    await signInAs(SEED_LOGINS.admin)
    const form = useCompanyForm({ mode: 'create' })
    fill(form)
    expect(await form.submit()).toBeNull()
    expect(form.formError.value).toMatch(/perfil não permite/)
  })
})

describe('useCompanyForm (edit)', () => {
  async function firstCompany(): Promise<Company> {
    const page = await useApiServices().companies.list()
    return page.items[0]!
  }

  it('fills from the company, locks the document and sends only changes', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const company = await firstCompany()
    const form = useCompanyForm({ mode: 'edit', company })
    expect(form.isDocumentLocked).toBe(true)
    expect(form.name.value).toBe(company.name)
    expect(form.document.value).toBe('12.345.678/0001-90')
    form.document.value = '99'
    expect(form.document.value).toBe('12.345.678/0001-90')

    expect(await form.submit()).toEqual({ mode: 'edit', company, changed: false })

    form.city.value = 'Muzambinho'
    const result = await form.submit()
    expect(result).toMatchObject({ mode: 'edit', changed: true, company: { id: company.id, city: 'Muzambinho', name: company.name } })
  })

  it('starts an evaluation when the plan changes to trial', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const company = await firstCompany()
    const form = useCompanyForm({ mode: 'edit', company })
    form.plan.value = 'trial'
    const result = await form.submit()
    expect(result?.company).toMatchObject({ plan: 'trial', status: 'trial' })
    expect(result?.company.trialEndsAt).toBeInstanceOf(Date)
  })

  it('fills itself when a deep-linked company arrives later', async () => {
    await signInAs(SEED_LOGINS.superAdmin)
    const company = await firstCompany()
    const source = ref<Company | null>(null)
    const form = useCompanyForm({ mode: 'edit', company: source })
    expect(form.isLoadingCompany.value).toBe(true)

    source.value = company
    await flushPromises()
    expect(form.isLoadingCompany.value).toBe(false)
    expect(form.name.value).toBe(company.name)
  })
})
