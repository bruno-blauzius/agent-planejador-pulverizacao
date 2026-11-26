import { afterEach, describe, expect, it, vi } from 'vitest'
import { useOwnerDocuments } from '~/composables/useOwnerDocuments'
import type { ApiServices } from '~/services/ports'
import type { OwnerInput } from '~/types/owner'
import { createTestApi } from '../../support/mockApi'
import { installNuxtFakes } from '../../support/nuxtFakes'

afterEach(() => vi.unstubAllGlobals())

const joana: OwnerInput = {
  personType: 'individual',
  name: 'Joana Lima',
  document: '390.533.447-05',
  birthDate: null,
  phone: null,
  email: null,
  windowAlertsEnabled: false,
}

async function newOwner(api: ApiServices) {
  const owner = await api.owners.create(joana)
  expect(owner.hasPendingDocuments).toBe(true)
  return owner.id
}

function pdf(name = 'rg.pdf'): File {
  return new File(['%PDF-1.7'], name, { type: 'application/pdf' })
}

describe('useOwnerDocuments', () => {
  it('sends an identity card, reloads the owner and clears the pending flag', async () => {
    const { api } = createTestApi()
    installNuxtFakes(api)
    const ownerId = await newOwner(api)
    const onChanged = vi.fn()
    const documents = useOwnerDocuments({ ownerId, onChanged })

    expect(documents.canUpload.value).toBe(false)
    documents.kind.value = 'id_card'
    documents.file.value = pdf()
    expect(documents.canUpload.value).toBe(true)

    const sent = await documents.upload()

    expect(sent?.fileName).toBe('rg.pdf')
    expect(onChanged).toHaveBeenCalledOnce()
    expect(documents.file.value).toBeNull()
    expect(documents.kind.value).toBeNull()
    expect((await api.owners.get(ownerId)).hasPendingDocuments).toBe(false)
  })

  it('explains a refused file and keeps the choice to try again', async () => {
    const { api } = createTestApi()
    installNuxtFakes(api)
    const ownerId = await newOwner(api)
    const onChanged = vi.fn()
    const documents = useOwnerDocuments({ ownerId, onChanged })
    documents.kind.value = 'id_card'
    documents.file.value = new File(['olá'], 'rg.txt', { type: 'text/plain' })

    expect(await documents.upload()).toBeNull()

    expect(documents.errorMessage.value).toBe('Formato não aceito. Envie PDF, JPEG ou PNG.')
    expect(documents.file.value?.name).toBe('rg.txt')
    expect(onChanged).not.toHaveBeenCalled()
  })

  it('removes a document and the owner becomes pending again', async () => {
    const { api } = createTestApi()
    installNuxtFakes(api)
    const ownerId = await newOwner(api)
    const onChanged = vi.fn()
    const documents = useOwnerDocuments({ ownerId, onChanged })
    documents.kind.value = 'driver_license'
    documents.file.value = pdf('cnh.pdf')
    const sent = await documents.upload()

    expect(await documents.remove(sent!.id)).toBe(true)

    expect(onChanged).toHaveBeenCalledTimes(2)
    expect(documents.removingId.value).toBeNull()
    expect((await api.owners.get(ownerId)).hasPendingDocuments).toBe(true)
    expect(await documents.remove(sent!.id)).toBe(false)
    expect(documents.errorMessage.value).toBe('O documento não existe mais. Atualize a página.')
  })
})
