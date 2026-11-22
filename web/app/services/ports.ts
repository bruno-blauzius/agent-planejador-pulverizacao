/**
 * Ports consumed by composables. Two adapters implement them: `http/` (services/api)
 * and `mock/` (in memory, ADR 0010). The choice is made only in `plugins/api.ts`.
 *
 * Every method rejects with `ApiError` on failure; none swallows errors.
 */
import type { AuthSession, Credentials, User } from '~/types/auth'
import type { Crop } from '~/types/crop'
import type { Farm, FarmFilters, FarmInput, FarmSummary, PerimeterImport } from '~/types/farm'
import type { Field, FieldInput } from '~/types/field'
import type {
  Owner,
  OwnerFarmLink,
  OwnerFarmLinkInput,
  OwnerFilters,
  OwnerInput,
  OwnerSummary,
} from '~/types/owner'
import type { Page } from '~/types/pagination'
import type {
  CompaniesSummary,
  Company,
  CompanyFilters,
  CompanyInput,
  CompanyUpdateInput,
  InvitationInput,
  UserAccount,
  UserFilters,
  UsersSummary,
  UserUpdateInput,
} from '~/types/admin'
import type { Dashboard, DashboardFilters } from '~/types/dashboard'
import type { PasswordChangeInput, Profile, ProfileInput } from '~/types/profile'

export interface AuthService {
  login(credentials: Credentials): Promise<AuthSession>
  getCurrentUser(): Promise<User>
}

export interface FarmService {
  list(filters?: FarmFilters): Promise<Page<FarmSummary>>
  get(farmId: string): Promise<Farm>
  create(input: FarmInput): Promise<Farm>
  update(farmId: string, input: FarmInput): Promise<Farm>
  importPerimeter(file: File): Promise<PerimeterImport>
}

export interface FieldService {
  listByFarm(farmId: string): Promise<readonly Field[]>
  create(farmId: string, input: FieldInput): Promise<Field>
  update(farmId: string, fieldId: string, input: FieldInput): Promise<Field>
}

export interface CropService {
  list(): Promise<readonly Crop[]>
}

export interface OwnerService {
  list(filters?: OwnerFilters): Promise<Page<OwnerSummary>>
  get(ownerId: string): Promise<Owner>
  create(input: OwnerInput): Promise<Owner>
  update(ownerId: string, input: OwnerInput): Promise<Owner>
  getLinks(ownerId: string): Promise<readonly OwnerFarmLink[]>
  replaceLinks(ownerId: string, links: readonly OwnerFarmLinkInput[]): Promise<readonly OwnerFarmLink[]>
}

/** Users and invitations (`users.manage`; 403 when missing). */
export interface UserService {
  list(filters?: UserFilters): Promise<Page<UserAccount>>
  summary(): Promise<UsersSummary>
  update(userId: string, input: UserUpdateInput): Promise<UserAccount>
  invite(input: InvitationInput): Promise<UserAccount>
  resendInvitation(invitationId: string): Promise<void>
  cancelInvitation(invitationId: string): Promise<void>
}

/** Client companies (`companies.manage`, super admin). */
export interface CompanyService {
  list(filters?: CompanyFilters): Promise<Page<Company>>
  summary(): Promise<CompaniesSummary>
  create(input: CompanyInput): Promise<Company>
  update(companyId: string, input: CompanyUpdateInput): Promise<Company>
}

/** Signed-in user's own profile (any role). */
export interface ProfileService {
  get(): Promise<Profile>
  update(input: ProfileInput): Promise<Profile>
  changePassword(input: PasswordChangeInput): Promise<void>
}

/** Administrator dashboard (`dashboard.view`). */
export interface DashboardService {
  get(filters?: DashboardFilters): Promise<Dashboard>
}

export interface ApiServices {
  readonly auth: AuthService
  readonly farms: FarmService
  readonly fields: FieldService
  readonly crops: CropService
  readonly owners: OwnerService
  readonly users: UserService
  readonly companies: CompanyService
  readonly profile: ProfileService
  readonly dashboard: DashboardService
}
