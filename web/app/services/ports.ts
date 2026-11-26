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
  FarmTerrain,
  FieldTerrain,
  FieldTerrainAdjustmentInput,
  TerrainCalculationScope,
} from '~/types/terrain'
import type {
  FieldTrafficability,
  RainGauge,
  RainGaugeInput,
  RainGaugePatch,
  RainReading,
  RainReadingInput,
  RainReadingRange,
  SoilInspection,
  SoilInspectionInput,
  SoilProfile,
  SoilProfileInput,
} from '~/types/trafficability'
import type {
  Owner,
  OwnerDocument,
  OwnerDocumentUpload,
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
  InvitationEmailStatus,
  InvitationInput,
  InvitationResult,
  UserAccount,
  UserFilters,
  UsersSummary,
  UserUpdateInput,
} from '~/types/admin'
import type { Dashboard, DashboardFilters } from '~/types/dashboard'
import type {
  ApplicationDetail,
  ApplicationFilters,
  ApplicationPage,
  CurrentWeather,
} from '~/types/application'
import type { PasswordChangeInput, Profile, ProfileInput } from '~/types/profile'
import type {
  ApplicationRegistrationInput,
  ApplicationReview,
  DayConditions,
  DayConditionsQuery,
  PlannerMessageInput,
  PlannerReply,
  RegisteredApplication,
  SprayWindowMap,
  TodayBrief,
  TodayTask,
  WindowMapQuery,
} from '~/types/planner'

interface BaseAuthService {
  /** The user of the session (the `agro_session` cookie, or the mock token). Rejects 401/403 (`ProductAccessCode`). */
  getCurrentUser(): Promise<User>
  /** Ends the product session on the server. Idempotent; rejects (e.g. 403 `forbidden`) when it could not. */
  logout(): Promise<void>
}

/** Real API (ADR 0015): login is a full-page redirect to the Keycloak of the Inceres SSO; the front never sees a password or token. */
export interface SsoAuthService extends BaseAuthService {
  readonly kind: 'sso'
  /** Absolute URL of `GET /auth/login`; `returnTo` is a relative path of this app. */
  loginUrl(returnTo: string): string
}

/** Dev mock only (ADR 0010): in-memory password login and Bearer token. */
export interface PasswordAuthService extends BaseAuthService {
  readonly kind: 'password'
  login(credentials: Credentials): Promise<AuthSession>
  changePassword(input: PasswordChangeInput): Promise<void>
}

export type AuthService = SsoAuthService | PasswordAuthService

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

/**
 * Relief of the fields (ADR 0021, tag `Terrain`, `planned`). Reading needs no
 * capability; calculating, adjusting and removing an adjustment need
 * `registry.write` (403).
 */
export interface TerrainService {
  /** `GET /farms/{farmId}/terrain`. Fields never calculated come as `not_calculated`. */
  getFarm(farmId: string): Promise<FarmTerrain>
  /** `POST /farms/{farmId}/terrain/calculations` (202). Repeating it while running returns the current state. */
  calculate(farmId: string, scope: TerrainCalculationScope): Promise<FarmTerrain>
  /** `GET /farms/{farmId}/fields/{fieldId}/terrain`. */
  getField(farmId: string, fieldId: string): Promise<FieldTerrain>
  /** `PUT .../terrain/adjustment`. 422 with `details` (`slope_max_pct`, `justification`, …). */
  adjust(farmId: string, fieldId: string, input: FieldTerrainAdjustmentInput): Promise<FieldTerrain>
  /** `DELETE .../terrain/adjustment`. 404 when the field has no adjustment. */
  removeAdjustment(farmId: string, fieldId: string): Promise<FieldTerrain>
}

/**
 * Rain, soil state and method eligibility of a field (ADR 0022, tag
 * `Trafficability`, `planned`). Read-only; any signed-in user in scope.
 */
export interface TrafficabilityService {
  /**
   * `GET /farms/{farmId}/fields/{fieldId}/trafficability?days=`. `days` (1–7) are
   * projection days after today; omitted, the API uses 5. 404 out of scope,
   * 422 with `details` (`days`) outside the range.
   */
  getFieldTrafficability(farmId: string, fieldId: string, days?: number): Promise<FieldTrafficability>
}

/**
 * Rain gauges of a farm and their readings (ADR 0022, phase 2). Reading needs
 * no capability; registering or renaming a gauge needs `registry.write` and
 * recording a reading `field_conditions.record` (403). A farm out of scope, or a
 * gauge of another farm, answers 404.
 */
export interface RainGaugeService {
  /** `GET /farms/{farmId}/rain-gauges`, by name. */
  list(farmId: string): Promise<readonly RainGauge[]>
  /** `POST /farms/{farmId}/rain-gauges` (201). 422 `name` when empty, too long or repeated in the farm. */
  create(farmId: string, input: RainGaugeInput): Promise<RainGauge>
  /** `PATCH .../rain-gauges/{gaugeId}`. 409 on a concurrent edit. */
  update(farmId: string, gaugeId: string, patch: RainGaugePatch): Promise<RainGauge>
  /** `GET .../readings?from&to`, newest first. 422 `from` when the range is inverted or longer than 31 days. */
  listReadings(farmId: string, gaugeId: string, range?: RainReadingRange): Promise<readonly RainReading[]>
  /** `POST .../readings` (201). 422 `rain_mm` (0–500) or `observed_at` (future, or a reading at the same instant). */
  recordReading(farmId: string, gaugeId: string, input: RainReadingInput): Promise<RainReading>
}

/** Soil inspection and soil profile of a field (ADR 0022, phase 2). */
export interface SoilConditionsService {
  /** `POST .../soil-inspections` (201, `field_conditions.record`). 422 `state`, `inspected_at` (future) or `note` (> 500). */
  recordInspection(farmId: string, fieldId: string, input: SoilInspectionInput): Promise<SoilInspection>
  /** `GET .../soil-profile`; texture and gauge are null until someone sets them. */
  getProfile(farmId: string, fieldId: string): Promise<SoilProfile>
  /** `PUT .../soil-profile` (`registry.write`). 422 `rain_gauge_id` for a gauge of another farm. */
  updateProfile(farmId: string, fieldId: string, input: SoilProfileInput): Promise<SoilProfile>
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
  /**
   * Uploads a document (PDF, JPEG or PNG, ADR 0020). 413 above the limit, 415
   * for another type (checked by content), 503 `document_storage_unavailable`.
   */
  uploadDocument(ownerId: string, upload: OwnerDocumentUpload): Promise<OwnerDocument>
  /** Soft delete; the owner's pending flag is recomputed by the API. */
  removeDocument(ownerId: string, documentId: string): Promise<void>
  /** Address of the file, for a download link (the session cookie goes along). */
  documentUrl(ownerId: string, documentId: string): string
}

/** Users and invitations (`users.manage`; 403 when missing). */
export interface UserService {
  list(filters?: UserFilters): Promise<Page<UserAccount>>
  summary(): Promise<UsersSummary>
  /** 404 for a user of another company (never 403), or deleted. */
  get(userId: string): Promise<UserAccount>
  update(userId: string, input: UserUpdateInput): Promise<UserAccount>
  /** Soft delete in the product; the realm account is untouched. 409 `invitation_pending`/`last_super_admin`. */
  remove(userId: string): Promise<void>
  /** 409 `email_already_registered`/`email_not_verified`/`company_inactive`; `emailStatus` says whether the e-mail left. */
  invite(input: InvitationInput): Promise<InvitationResult>
  /** 409 `invitation_closed`/`company_inactive`; resolves whether the e-mail left. */
  resendInvitation(invitationId: string): Promise<InvitationEmailStatus>
  /** 409 `invitation_closed`. */
  cancelInvitation(invitationId: string): Promise<void>
}

/** Client companies (`companies.manage`, super admin). */
export interface CompanyService {
  list(filters?: CompanyFilters): Promise<Page<Company>>
  summary(): Promise<CompaniesSummary>
  /** The platform operator company and deleted ones are 404. */
  get(companyId: string): Promise<Company>
  /** 409 `conflict` for a duplicated CNPJ; 422 for an invalid one. */
  create(input: CompanyInput): Promise<Company>
  /** `status: 'inactive'` cuts every user of the company at once; 502 `identity_provider_unavailable` is safe to retry. */
  update(companyId: string, input: CompanyUpdateInput): Promise<Company>
  /** Soft delete; 409 `company_has_active_users` while users are active or invited. */
  remove(companyId: string): Promise<void>
}

/** Signed-in user's own profile (any role). Only the name is editable; the password is Keycloak's (ADR 0015). */
export interface ProfileService {
  get(): Promise<Profile>
  update(input: ProfileInput): Promise<Profile>
}

/** Administrator dashboard (`dashboard.view`). */
export interface DashboardService {
  get(filters?: DashboardFilters): Promise<Dashboard>
}

/** Spray planner chat, window map and hourly conditions (`planner.use`; ADR 0016). */
export interface PlannerService {
  /** One round trip, no streaming. Rejects `agent_unavailable`/`weather_unavailable` (502) when a dependency is down. */
  sendMessage(input: PlannerMessageInput): Promise<PlannerReply>
  getWindowMap(query: WindowMapQuery): Promise<SprayWindowMap>
  getDayConditions(query: DayConditionsQuery): Promise<DayConditions>
}

/** Applications: human-confirmed registration (`applications.register`) and history (scope only; ADR 0016). */
export interface ApplicationService {
  /** Applications of the user's farms, planned first (API order); the facets feed the filter selects. */
  list(filters?: ApplicationFilters): Promise<ApplicationPage>
  get(applicationId: string): Promise<ApplicationDetail>
  /** Recomputes the checks now; does not write an application. */
  review(planId: string, fieldId: string): Promise<ApplicationReview>
  /** Called only after the explicit confirmation; rejects `conditions_changed` (409) when the review is stale. */
  register(input: ApplicationRegistrationInput): Promise<RegisteredApplication>
}

/** Operator "Hoje" screen (`planner.use`; starting a task needs `applications.register`). */
export interface TodayService {
  getBrief(): Promise<TodayBrief>
  startTask(taskId: string): Promise<TodayTask>
}

/** Current weather chip of the farm (any signed-in user in scope). */
export interface WeatherService {
  /** Without `farmId`, the user's primary farm. Rejects `weather_unavailable` (502) when the forecast is down. */
  getCurrent(farmId?: string): Promise<CurrentWeather>
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
  readonly planner: PlannerService
  readonly applications: ApplicationService
  readonly today: TodayService
  readonly weather: WeatherService
  readonly terrain: TerrainService
  readonly trafficability: TrafficabilityService
  readonly rainGauges: RainGaugeService
  readonly soilConditions: SoilConditionsService
}
