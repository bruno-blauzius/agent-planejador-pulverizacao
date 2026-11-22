/** HTTP adapters of the ports, against services/api (api/openapi.yaml). */
import type {
  ApiServices,
  AuthService,
  CropService,
  FarmService,
  FieldService,
  OwnerService,
} from '../ports'
import type {
  CropDto,
  FarmDto,
  FarmSummaryDto,
  FieldDto,
  ItemsDto,
  LoginResponseDto,
  OwnerDto,
  OwnerFarmLinkDto,
  OwnerSummaryDto,
  PageDto,
  PerimeterImportDto,
  UserDto,
} from '~/types/dto'
import type { HttpClient } from './client'
import {
  createCompanyHttpService,
  createDashboardHttpService,
  createProfileHttpService,
  createUserHttpService,
} from './adminServices'
import {
  toAuthSession,
  toFarm,
  toFarmInputDto,
  toFarmSummary,
  toField,
  toFieldInputDto,
  toLoginRequestDto,
  toOwner,
  toOwnerFarmLink,
  toOwnerFarmLinkInputDto,
  toOwnerInputDto,
  toOwnerSummary,
  toPage,
  toPerimeterImport,
  toUser,
} from './mappers'

const segment = encodeURIComponent

export function createAuthHttpService(http: HttpClient): AuthService {
  return {
    async login(credentials) {
      const dto = await http<LoginResponseDto>('/auth/login', { method: 'POST', body: toLoginRequestDto(credentials) })
      return toAuthSession(dto)
    },
    async getCurrentUser() {
      return toUser(await http<UserDto>('/auth/me'))
    },
  }
}

export function createFarmHttpService(http: HttpClient): FarmService {
  return {
    async list(filters = {}) {
      const dto = await http<PageDto<FarmSummaryDto>>('/farms', {
        query: {
          search: filters.search,
          status: filters.status,
          production_type: filters.productionType,
          page: filters.page,
          page_size: filters.pageSize,
        },
      })
      return toPage(dto, toFarmSummary)
    },
    async get(farmId) {
      return toFarm(await http<FarmDto>(`/farms/${segment(farmId)}`))
    },
    async create(input) {
      return toFarm(await http<FarmDto>('/farms', { method: 'POST', body: toFarmInputDto(input) }))
    },
    async update(farmId, input) {
      const dto = await http<FarmDto>(`/farms/${segment(farmId)}`, { method: 'PUT', body: toFarmInputDto(input) })
      return toFarm(dto)
    },
    async importPerimeter(file) {
      const body = new FormData()
      body.append('file', file)
      const dto = await http<PerimeterImportDto>('/perimeter-imports', { method: 'POST', body })
      return toPerimeterImport(dto)
    },
  }
}

export function createFieldHttpService(http: HttpClient): FieldService {
  return {
    async listByFarm(farmId) {
      const dto = await http<ItemsDto<FieldDto>>(`/farms/${segment(farmId)}/fields`)
      return dto.items.map(toField)
    },
    async create(farmId, input) {
      const dto = await http<FieldDto>(`/farms/${segment(farmId)}/fields`, {
        method: 'POST',
        body: toFieldInputDto(input),
      })
      return toField(dto)
    },
    async update(farmId, fieldId, input) {
      const dto = await http<FieldDto>(`/farms/${segment(farmId)}/fields/${segment(fieldId)}`, {
        method: 'PUT',
        body: toFieldInputDto(input),
      })
      return toField(dto)
    },
  }
}

export function createCropHttpService(http: HttpClient): CropService {
  return {
    async list() {
      return (await http<ItemsDto<CropDto>>('/crops')).items
    },
  }
}

export function createOwnerHttpService(http: HttpClient): OwnerService {
  return {
    async list(filters = {}) {
      const dto = await http<PageDto<OwnerSummaryDto>>('/owners', {
        query: {
          search: filters.search,
          person_type: filters.personType,
          page: filters.page,
          page_size: filters.pageSize,
        },
      })
      return toPage(dto, toOwnerSummary)
    },
    async get(ownerId) {
      return toOwner(await http<OwnerDto>(`/owners/${segment(ownerId)}`))
    },
    async create(input) {
      return toOwner(await http<OwnerDto>('/owners', { method: 'POST', body: toOwnerInputDto(input) }))
    },
    async update(ownerId, input) {
      const dto = await http<OwnerDto>(`/owners/${segment(ownerId)}`, { method: 'PUT', body: toOwnerInputDto(input) })
      return toOwner(dto)
    },
    async getLinks(ownerId) {
      const dto = await http<ItemsDto<OwnerFarmLinkDto>>(`/owners/${segment(ownerId)}/links`)
      return dto.items.map(toOwnerFarmLink)
    },
    async replaceLinks(ownerId, links) {
      const dto = await http<ItemsDto<OwnerFarmLinkDto>>(`/owners/${segment(ownerId)}/links`, {
        method: 'PUT',
        body: { links: links.map(toOwnerFarmLinkInputDto) },
      })
      return dto.items.map(toOwnerFarmLink)
    },
  }
}

export function createHttpServices(http: HttpClient): ApiServices {
  return {
    auth: createAuthHttpService(http),
    farms: createFarmHttpService(http),
    fields: createFieldHttpService(http),
    crops: createCropHttpService(http),
    owners: createOwnerHttpService(http),
    users: createUserHttpService(http),
    companies: createCompanyHttpService(http),
    profile: createProfileHttpService(http),
    dashboard: createDashboardHttpService(http),
  }
}
