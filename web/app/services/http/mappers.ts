/** DTO (snake_case, api/openapi.yaml) <-> front model. Pure functions. */
import type { AuthSession, Credentials, FarmScope, User } from '~/types/auth'
import type {
  FarmDto,
  FarmInputDto,
  FarmLocationDto,
  FarmOwnerShareDto,
  FarmSummaryDto,
  FieldDto,
  FieldInputDto,
  LoginRequestDto,
  LoginResponseDto,
  OwnerDocumentDto,
  OwnerDto,
  OwnerFarmLinkDto,
  OwnerFarmLinkInputDto,
  OwnerInputDto,
  OwnerSummaryDto,
  PageDto,
  PerimeterImportDto,
  UserDto,
  FarmScopeDto,
} from '~/types/dto'
import type { Farm, FarmInput, FarmLocation, FarmOwnerShare, FarmSummary, PerimeterImport } from '~/types/farm'
import type { Field, FieldInput } from '~/types/field'
import type {
  Owner,
  OwnerDocument,
  OwnerFarmLink,
  OwnerFarmLinkInput,
  OwnerInput,
  OwnerSummary,
} from '~/types/owner'
import type { Page } from '~/types/pagination'

export function toPage<D, M>(dto: PageDto<D>, mapItem: (item: D) => M): Page<M> {
  return { items: dto.items.map(mapItem), total: dto.total, page: dto.page, pageSize: dto.page_size }
}

export function toFarmScope(dto: FarmScopeDto): FarmScope {
  return dto.mode === 'all' ? { mode: 'all' } : { mode: 'selected', farmIds: dto.farm_ids }
}

export function toFarmScopeDto(scope: FarmScope): FarmScopeDto {
  return scope.mode === 'all' ? { mode: 'all' } : { mode: 'selected', farm_ids: scope.farmIds }
}

export function toUser(dto: UserDto): User {
  return {
    id: dto.id,
    name: dto.name,
    email: dto.email,
    role: dto.role,
    primaryFarm: dto.primary_farm,
    company: dto.company,
    farmScope: toFarmScope(dto.farm_scope),
    permissions: dto.permissions,
  }
}

export function toLoginRequestDto(credentials: Credentials): LoginRequestDto {
  return {
    identifier: credentials.identifier.trim(),
    password: credentials.password,
    remember_me: credentials.rememberMe,
  }
}

export function toAuthSession(dto: LoginResponseDto): AuthSession {
  return { accessToken: dto.access_token, expiresAt: new Date(dto.expires_at), user: toUser(dto.user) }
}

function toFarmOwnerShare(dto: FarmOwnerShareDto): FarmOwnerShare {
  return { ownerId: dto.owner_id, name: dto.name, role: dto.role, sharePercent: dto.share_percent }
}

export function toFarmSummary(dto: FarmSummaryDto): FarmSummary {
  return {
    companyId: dto.company_id,
    id: dto.id,
    name: dto.name,
    code: dto.code,
    city: dto.city,
    state: dto.state,
    status: dto.status,
    productionType: dto.production_type,
    totalAreaHa: dto.total_area_ha,
    productiveAreaHa: dto.productive_area_ha,
    fieldCount: dto.field_count,
    crops: dto.crops,
    owners: dto.owners.map(toFarmOwnerShare),
    sprayOutlook: dto.spray_outlook,
    forecastPoint: dto.forecast_point,
    perimeter: dto.perimeter,
    createdAt: new Date(dto.created_at),
  }
}

function toFarmLocation(dto: FarmLocationDto): FarmLocation {
  return {
    postalCode: dto.postal_code,
    carNumber: dto.car_number,
    forecastPoint: dto.forecast_point,
    perimeter: dto.perimeter,
  }
}

export function toFarm(dto: FarmDto): Farm {
  return {
    ...toFarmSummary(dto),
    cooperative: dto.cooperative,
    location: toFarmLocation(dto.location),
    updatedAt: new Date(dto.updated_at),
  }
}

export function toFarmInputDto(input: FarmInput): FarmInputDto {
  return {
    name: input.name,
    code: input.code,
    cooperative: input.cooperative,
    production_type: input.productionType,
    city: input.city,
    state: input.state,
    status: input.status,
    location: {
      postal_code: input.location.postalCode,
      car_number: input.location.carNumber,
      forecast_point: input.location.forecastPoint,
      perimeter: input.location.perimeter,
    },
    owners: input.owners.map(owner => ({
      owner_id: owner.ownerId,
      role: owner.role,
      share_percent: owner.sharePercent,
    })),
  }
}

export function toPerimeterImport(dto: PerimeterImportDto): PerimeterImport {
  return {
    sourceFormat: dto.source_format,
    perimeter: dto.perimeter,
    areaHa: dto.area_ha,
    centroid: dto.centroid,
    city: dto.municipality ?? null,
    state: dto.state ?? null,
    datum: dto.datum ?? null,
    fieldCount: dto.field_count ?? null,
  }
}

export function toField(dto: FieldDto): Field {
  return {
    id: dto.id,
    farmId: dto.farm_id,
    name: dto.name,
    block: dto.block,
    areaHa: dto.area_ha,
    crop: dto.crop,
    variety: dto.variety,
    season: dto.season,
    plantingDate: dto.planting_date,
    rowSpacingM: dto.row_spacing_m,
    plantSpacingM: dto.plant_spacing_m,
    plantsPerHa: dto.plants_per_ha,
    polygon: dto.polygon,
    sprayCondition: dto.spray_condition,
    lastApplicationAt: dto.last_application_at ? new Date(dto.last_application_at) : null,
  }
}

export function toFieldInputDto(input: FieldInput): FieldInputDto {
  return {
    name: input.name,
    block: input.block,
    crop_code: input.cropCode,
    variety: input.variety,
    season: input.season,
    planting_date: input.plantingDate,
    row_spacing_m: input.rowSpacingM,
    plant_spacing_m: input.plantSpacingM,
    polygon: input.polygon,
  }
}

export function toOwnerFarmLink(dto: OwnerFarmLinkDto): OwnerFarmLink {
  return { farmId: dto.farm_id, farmName: dto.farm_name, role: dto.role, sharePercent: dto.share_percent }
}

export function toOwnerFarmLinkInputDto(input: OwnerFarmLinkInput): OwnerFarmLinkInputDto {
  return { farm_id: input.farmId, role: input.role, share_percent: input.sharePercent }
}

function toOwnerDocument(dto: OwnerDocumentDto): OwnerDocument {
  return { id: dto.id, kind: dto.kind, fileName: dto.file_name, uploadedAt: new Date(dto.uploaded_at) }
}

export function toOwnerSummary(dto: OwnerSummaryDto): OwnerSummary {
  return {
    id: dto.id,
    personType: dto.person_type,
    name: dto.name,
    document: dto.document,
    hasPendingDocuments: dto.has_pending_documents,
    links: dto.links.map(toOwnerFarmLink),
  }
}

export function toOwner(dto: OwnerDto): Owner {
  return {
    ...toOwnerSummary(dto),
    birthDate: dto.birth_date,
    phone: dto.phone,
    email: dto.email,
    windowAlertsEnabled: dto.window_alerts_enabled,
    documents: dto.documents.map(toOwnerDocument),
    createdAt: new Date(dto.created_at),
    updatedAt: new Date(dto.updated_at),
  }
}

export function toOwnerInputDto(input: OwnerInput): OwnerInputDto {
  return {
    person_type: input.personType,
    name: input.name,
    document: input.document,
    birth_date: input.birthDate,
    phone: input.phone,
    email: input.email,
    window_alerts_enabled: input.windowAlertsEnabled,
  }
}
