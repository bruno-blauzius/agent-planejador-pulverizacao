import { describe, expect, it } from 'vitest'
import { ApiError } from '~/services/errors'
import type { Company } from '~/types/admin'
import {
  companyActionErrorMessage,
  companyApiFieldErrors,
  companyConfirmationCopy,
  companyFormErrorMessage,
  companyRowActions,
  companyUpdateInput,
  emptyCompanyForm,
  maskCnpjInput,
  maskStateInput,
  reactivationStatus,
  validateCompanyForm,
} from '~/utils/companyForm'

const company: Company = {
  id: 'c-1',
  name: 'Fazenda Santa Rita',
  document: '11222333000181',
  city: 'Guaxupé',
  state: 'MG',
  plan: 'pro',
  status: 'active',
  trialEndsAt: null,
  farmCount: 2,
  userCount: 3,
  createdAt: new Date('2026-01-01T00:00:00Z'),
}

function apiError(code: ConstructorParameters<typeof ApiError>[0]['code'], details: { field: string, message: string }[] = []) {
  return new ApiError({ code, message: 'texto do servidor', status: 422, details })
}

describe('company form masks', () => {
  it('masks the CNPJ progressively and caps at 14 digits', () => {
    expect(maskCnpjInput('11')).toBe('11')
    expect(maskCnpjInput('112223330001')).toBe('11.222.333/0001')
    expect(maskCnpjInput('11222333000181999')).toBe('11.222.333/0001-81')
  })

  it('keeps only 2 upper-case letters in the UF', () => {
    expect(maskStateInput('mg1x')).toBe('MG')
    expect(maskStateInput('1')).toBe('')
  })
})

describe('validateCompanyForm', () => {
  it('flags every empty field of a new company', () => {
    expect(Object.keys(validateCompanyForm(emptyCompanyForm())).sort()).toEqual(['city', 'document', 'name', 'state'])
  })

  it('checks the CNPJ check digits', () => {
    const base = { ...emptyCompanyForm(), name: 'X', city: 'Y', state: 'MG' }
    expect(validateCompanyForm({ ...base, document: '11.222.333/0001-81' })).toEqual({})
    expect(validateCompanyForm({ ...base, document: '11.222.333/0001-82' }).document).toBe('CNPJ inválido.')
    expect(validateCompanyForm({ ...base, document: '11.222' }).document).toBe('CNPJ inválido.')
  })

  it('skips the document when it is locked (edit)', () => {
    expect(validateCompanyForm({ ...emptyCompanyForm(), name: 'X', city: 'Y', state: 'MG' }, { skipDocument: true })).toEqual({})
  })
})

describe('API errors on the form', () => {
  it('maps a duplicated CNPJ (409 conflict) to the document field', () => {
    const error = apiError('conflict')
    expect(companyApiFieldErrors(error)).toEqual({ document: 'Já existe uma empresa com este CNPJ.' })
    expect(companyFormErrorMessage(error)).toBeNull()
  })

  it('maps 422 details by field and ignores unknown ones', () => {
    const error = apiError('validation_failed', [{ field: 'document', message: 'CNPJ inválido' }, { field: 'x', message: '?' }])
    expect(companyApiFieldErrors(error)).toEqual({ document: 'CNPJ inválido' })
  })

  it('uses the footer for what no field explains', () => {
    expect(companyFormErrorMessage(apiError('forbidden'))).toMatch(/perfil não permite/)
    expect(companyFormErrorMessage(apiError('network_error'))).toMatch(/conectar/)
    expect(companyFormErrorMessage(apiError('unknown_error'))).toBe('Não foi possível salvar. Tente novamente.')
    expect(companyFormErrorMessage(null)).toBeNull()
  })
})

describe('companyUpdateInput', () => {
  const values = { name: company.name, document: '', city: company.city, state: company.state, plan: company.plan }

  it('sends only what changed', () => {
    expect(companyUpdateInput(values, company)).toEqual({})
    expect(companyUpdateInput({ ...values, name: '  Nova  ', state: 'sp' }, company)).toEqual({ name: 'Nova', state: 'SP' })
    expect(companyUpdateInput({ ...values, plan: 'trial' }, company)).toEqual({ plan: 'trial' })
  })
})

describe('row actions and copy', () => {
  it('offers delete only for inactive companies', () => {
    expect(companyRowActions('active').map(action => action.kind)).toEqual(['edit', 'deactivate'])
    expect(companyRowActions('trial').map(action => action.kind)).toEqual(['edit', 'deactivate'])
    expect(companyRowActions('inactive').map(action => action.kind)).toEqual(['edit', 'reactivate', 'delete'])
  })

  it('names the company in the confirmations', () => {
    expect(companyConfirmationCopy('deactivate', company).title).toBe('Inativar Fazenda Santa Rita?')
    expect(companyConfirmationCopy('delete', company)).toMatchObject({ title: 'Excluir Fazenda Santa Rita?', tone: 'danger' })
  })

  it('reactivates a trial company back to trial only while the period is known', () => {
    expect(reactivationStatus(company)).toBe('active')
    expect(reactivationStatus({ ...company, plan: 'trial', trialEndsAt: new Date() })).toBe('trial')
    expect(reactivationStatus({ ...company, plan: 'trial', trialEndsAt: null })).toBe('active')
  })

  it('turns contract errors into actionable pt-BR text', () => {
    expect(companyActionErrorMessage(apiError('company_has_active_users'), 'delete')).toMatch(/usuários ativos ou convidados/)
    expect(companyActionErrorMessage(apiError('identity_provider_unavailable'), 'deactivate')).toMatch(/inativada/)
    expect(companyActionErrorMessage(apiError('identity_provider_unavailable'), 'reactivate')).toMatch(/SSO/)
    expect(companyActionErrorMessage(apiError('not_found'), 'delete')).toBe('A empresa não existe mais.')
  })
})
