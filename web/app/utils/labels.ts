/** pt-BR labels of contract enums, as written in the Figma screens. */
import type { UserRole } from '~/types/auth'
import type { FarmStatus, ProductionType } from '~/types/farm'
import type { LinkRole, PersonType } from '~/types/owner'
import type { SprayCondition } from '~/types/spray'

export const FARM_STATUS_LABELS: Readonly<Record<FarmStatus, string>> = {
  draft: 'Rascunho',
  active: 'Ativa',
  pending: 'Com pendências',
}

export const PRODUCTION_TYPE_LABELS: Readonly<Record<ProductionType, string>> = {
  coffee: 'Café',
  grains: 'Grãos',
  livestock: 'Pecuária',
  mixed: 'Misto',
}

export const SPRAY_CONDITION_LABELS: Readonly<Record<SprayCondition, string>> = {
  suitable: 'Apta',
  attention: 'Atenção',
  unsuitable: 'Inapta',
  rain: 'Chuva',
}

export const PERSON_TYPE_LABELS: Readonly<Record<PersonType, string>> = {
  individual: 'Pessoa física',
  company: 'Pessoa jurídica',
}

export const PERSON_TYPE_SHORT_LABELS: Readonly<Record<PersonType, string>> = {
  individual: 'PF',
  company: 'PJ',
}

export const LINK_ROLE_LABELS: Readonly<Record<LinkRole, string>> = {
  owner: 'Proprietário',
  tenant: 'Arrendatário',
  legal_representative: 'Responsável legal',
}

export const USER_ROLE_LABELS: Readonly<Record<UserRole, string>> = {
  super_admin: 'Super admin',
  admin: 'Administrador',
  planning_operator: 'Operador de planejamento',
}
