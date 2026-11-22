/**
 * In-memory adapters of page 03 (ADR 0013): users and invitations, companies,
 * profile and dashboard. Same rules the API will apply: capability per
 * endpoint, company scope, 403 `forbidden`/`out_of_scope`, 404 for other tenants.
 */
import type { FieldError } from '../errors'
import type { CompanyService, DashboardService, ProfileService, UserService } from '../ports'
import type { Company, UserAccount } from '~/types/admin'
import type { FarmScope, Permission, UserRole } from '~/types/auth'
import type { Dashboard, UpcomingWindow } from '~/types/dashboard'
import type { Profile } from '~/types/profile'
import { canSeeFarm, farmAccess, type Viewer } from './access'
import type { CompanyRecord, MockSeed, UserRecord } from './seed'
import { digitsOf, fail, failIfInvalid, normalize, paginate, roundOneDecimal } from './support'

export interface AdminMockContext {
  readonly db: MockSeed
  readonly now: () => Date
  readonly nextId: (prefix: string) => string
  readonly authorized: <T>(produce: () => T, permission?: Permission) => Promise<T>
  readonly currentViewer: () => Viewer
  readonly currentUserRecord: () => UserRecord
}

const INVITATION_DAYS = 7
const TRIAL_DAYS = 14
const DAY_MS = 86_400_000
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const PASSWORD_MIN_LENGTH = 8

export function createAdminMockServices(ctx: AdminMockContext): {
  users: UserService
  companies: CompanyService
  profile: ProfileService
  dashboard: DashboardService
} {
  const { db, now, nextId, authorized, currentViewer } = ctx

  const companyName = (companyId: string) => db.companies.find(item => item.id === companyId)?.name ?? ''
  const managesAll = () => currentViewer().permissions.includes('users.manage_all')

  // ---- users and invitations ----------------------------------------------

  function toUserAccount(record: UserRecord): UserAccount {
    return {
      id: record.id,
      name: record.name,
      email: record.email,
      role: record.role,
      company: { id: record.companyId, name: companyName(record.companyId) },
      farmScope: record.farmScope,
      status: record.status,
      lastAccessAt: record.lastAccessAt,
      invitation: record.invitation,
    }
  }

  /** Users the caller manages: all (super admin) or the caller's company. */
  function managedUsers(): UserRecord[] {
    return managesAll() ? db.users : db.users.filter(item => item.companyId === currentViewer().companyId)
  }

  function findManagedUser(predicate: (record: UserRecord) => boolean, message: string): UserRecord {
    return managedUsers().find(predicate) ?? fail('not_found', message)
  }

  /** Admins cannot grant super admin nor move people to other companies. */
  function checkAssignment(role: UserRole | undefined, companyId: string | undefined, target?: UserRecord) {
    if (managesAll()) return
    if (role === 'super_admin' || target?.role === 'super_admin') fail('forbidden', 'apenas o super admin gerencia esse papel')
    if (companyId !== undefined && companyId !== currentViewer().companyId) fail('forbidden', 'você só gerencia usuários da sua empresa')
  }

  function validateScope(scope: FarmScope | undefined, companyId: string, details: FieldError[]) {
    if (scope?.mode !== 'selected') return
    if (scope.farmIds.length === 0) details.push({ field: 'farm_scope.farm_ids', message: 'selecione ao menos uma fazenda' })
    if (scope.farmIds.some(farmId => db.farms.find(farm => farm.id === farmId)?.companyId !== companyId)) {
      details.push({ field: 'farm_scope.farm_ids', message: 'fazenda não pertence à empresa' })
    }
  }

  const users: UserService = {
    list(filters = {}) {
      return authorized(() => {
        const search = normalize(filters.search ?? '')
        const matches = managedUsers()
          .filter(item => !filters.companyId || !managesAll() || item.companyId === filters.companyId)
          .filter(item => !filters.role || item.role === filters.role)
          .filter(item => !filters.status || item.status === filters.status)
          .filter(item => !search || [item.name, item.email, companyName(item.companyId)].some(text => normalize(text).includes(search)))
          .map(toUserAccount)
        return paginate(matches, filters)
      }, 'users.manage')
    },
    summary() {
      return authorized(() => {
        const scoped = managedUsers()
        const byRole = { super_admin: 0, admin: 0, planning_operator: 0 }
        scoped.forEach(item => (byRole[item.role] += 1))
        return {
          totalUsers: scoped.length,
          companyCount: new Set(scoped.map(item => item.companyId)).size,
          pendingInvitations: scoped.filter(item => item.status === 'invited').length,
          byRole,
        }
      }, 'users.manage')
    },
    update(userId, input) {
      return authorized(() => {
        const record = findManagedUser(item => item.id === userId, 'usuário não encontrado')
        checkAssignment(input.role, input.companyId, record)
        if (record.id === currentViewer().id && (input.role || input.status === 'inactive')) {
          fail('forbidden', 'você não pode alterar o próprio papel ou desativar a própria conta')
        }
        const companyId = input.companyId ?? record.companyId
        const details: FieldError[] = []
        if (!db.companies.some(item => item.id === companyId)) details.push({ field: 'company_id', message: 'empresa não encontrada' })
        validateScope(input.farmScope, companyId, details)
        if (input.status && record.status === 'invited') details.push({ field: 'status', message: 'convite pendente; reenvie ou cancele o convite' })
        failIfInvalid(details)
        Object.assign(record, {
          role: input.role ?? record.role,
          companyId,
          farmScope: input.farmScope ?? (input.companyId && input.companyId !== record.companyId ? { mode: 'all' } : record.farmScope),
          status: input.status ?? record.status,
        })
        return toUserAccount(record)
      }, 'users.manage')
    },
    invite(input) {
      return authorized(() => {
        checkAssignment(input.role, input.companyId)
        const details: FieldError[] = []
        if (!input.name.trim()) details.push({ field: 'name', message: 'informe o nome' })
        if (!EMAIL_PATTERN.test(input.email.trim())) details.push({ field: 'email', message: 'e-mail inválido' })
        if (!db.companies.some(item => item.id === input.companyId)) details.push({ field: 'company_id', message: 'empresa não encontrada' })
        validateScope(input.farmScope, input.companyId, details)
        failIfInvalid(details)
        if (db.users.some(item => normalize(item.email) === normalize(input.email))) fail('conflict', 'e-mail já cadastrado')
        const record: UserRecord = {
          id: nextId('user'),
          name: input.name.trim(),
          email: input.email.trim(),
          document: '',
          password: null,
          role: input.role,
          companyId: input.companyId,
          farmScope: input.farmScope,
          status: 'invited',
          lastAccessAt: null,
          invitation: { id: nextId('invitation'), expiresAt: new Date(now().getTime() + INVITATION_DAYS * DAY_MS) },
          primaryFarmId: null,
          passwordChangedAt: null,
        }
        db.users.push(record)
        return toUserAccount(record)
      }, 'users.manage')
    },
    resendInvitation(invitationId) {
      return authorized(() => {
        const record = findManagedUser(item => item.invitation?.id === invitationId, 'convite não encontrado')
        record.invitation = { id: invitationId, expiresAt: new Date(now().getTime() + INVITATION_DAYS * DAY_MS) }
      }, 'users.manage')
    },
    cancelInvitation(invitationId) {
      return authorized(() => {
        const record = findManagedUser(item => item.invitation?.id === invitationId, 'convite não encontrado')
        db.users.splice(db.users.indexOf(record), 1)
      }, 'users.manage')
    },
  }

  // ---- companies ----------------------------------------------------------

  const clientCompanies = () => db.companies.filter(item => !item.isPlatformOperator)

  function toCompany(record: CompanyRecord): Company {
    return {
      id: record.id,
      name: record.name,
      document: record.document,
      city: record.city,
      state: record.state,
      plan: record.plan,
      status: record.status,
      trialEndsAt: record.trialEndsAt,
      farmCount: db.farms.filter(farm => farm.companyId === record.id).length,
      userCount: db.users.filter(item => item.companyId === record.id).length,
      createdAt: record.createdAt,
    }
  }

  function findClientCompany(companyId: string): CompanyRecord {
    return clientCompanies().find(item => item.id === companyId) ?? fail('not_found', 'empresa não encontrada')
  }

  function validateCompany(fields: { name?: string, document?: string, state?: string }, companyId: string | null): string | undefined {
    const details: FieldError[] = []
    const document = fields.document === undefined ? undefined : digitsOf(fields.document)
    if (fields.name !== undefined && !fields.name.trim()) details.push({ field: 'name', message: 'informe o nome' })
    if (document !== undefined && document.length !== 14) details.push({ field: 'document', message: 'CNPJ inválido' })
    if (fields.state !== undefined && fields.state.trim().length !== 2) details.push({ field: 'state', message: 'UF deve ter 2 letras' })
    failIfInvalid(details)
    if (document && db.companies.some(item => item.document === document && item.id !== companyId)) fail('conflict', 'CNPJ já cadastrado')
    return document
  }

  const companies: CompanyService = {
    list(filters = {}) {
      return authorized(() => {
        const search = normalize(filters.search ?? '')
        const searchDigits = digitsOf(search)
        const matches = clientCompanies()
          .filter(item => !filters.status || item.status === filters.status)
          .filter(item => !search
            || [item.name, item.city].some(text => normalize(text).includes(search))
            || (searchDigits.length > 0 && item.document.includes(searchDigits)))
          .map(toCompany)
        return paginate(matches, filters)
      }, 'companies.manage')
    },
    summary() {
      return authorized(() => {
        const clients = clientCompanies()
        return {
          activeCompanies: clients.filter(item => item.status !== 'inactive').length,
          trialCompanies: clients.filter(item => item.status === 'trial').length,
          farmCount: db.farms.length,
          monitoredAreaHa: roundOneDecimal(db.farms.reduce((sum, farm) => sum + farm.totalAreaHa, 0)),
          userCount: db.users.length,
          pendingInvitations: db.users.filter(item => item.status === 'invited').length,
          applicationsThisMonth: clients.reduce((sum, item) => sum + item.applicationsThisMonth, 0),
          applicationsPreviousMonth: clients.reduce((sum, item) => sum + item.applicationsPreviousMonth, 0),
        }
      }, 'companies.manage')
    },
    create(input) {
      return authorized(() => {
        const document = validateCompany(input, null) ?? ''
        const isTrial = input.plan === 'trial'
        const record: CompanyRecord = {
          id: nextId('company'),
          name: input.name.trim(),
          document,
          city: input.city.trim(),
          state: input.state.trim().toUpperCase(),
          plan: input.plan,
          status: isTrial ? 'trial' : 'active',
          trialEndsAt: isTrial ? new Date(now().getTime() + TRIAL_DAYS * DAY_MS) : null,
          createdAt: now(),
          isPlatformOperator: false,
          applicationsThisMonth: 0,
          applicationsPreviousMonth: 0,
        }
        db.companies.push(record)
        return toCompany(record)
      }, 'companies.manage')
    },
    update(companyId, input) {
      return authorized(() => {
        const record = findClientCompany(companyId)
        validateCompany(input, companyId)
        Object.assign(record, {
          name: input.name?.trim() ?? record.name,
          city: input.city?.trim() ?? record.city,
          state: input.state?.trim().toUpperCase() ?? record.state,
          plan: input.plan ?? record.plan,
          status: input.status ?? record.status,
        })
        return toCompany(record)
      }, 'companies.manage')
    },
  }

  // ---- profile ------------------------------------------------------------

  function toProfile(record: UserRecord): Profile {
    return {
      id: record.id,
      name: record.name,
      email: record.email,
      role: record.role,
      company: { id: record.companyId, name: companyName(record.companyId) },
      farmScope: record.farmScope,
      passwordChangedAt: record.passwordChangedAt,
    }
  }

  const profile: ProfileService = {
    get() {
      return authorized(() => toProfile(ctx.currentUserRecord()))
    },
    update(input) {
      return authorized(() => {
        if (!input.name.trim()) fail('validation_failed', 'dados inválidos', [{ field: 'name', message: 'informe o nome' }])
        const record = ctx.currentUserRecord()
        record.name = input.name.trim()
        return toProfile(record)
      })
    },
    changePassword(input) {
      return authorized(() => {
        const record = ctx.currentUserRecord()
        const details: FieldError[] = []
        if (record.password !== input.currentPassword) details.push({ field: 'current_password', message: 'senha atual incorreta' })
        const next = input.newPassword
        if (next.length < PASSWORD_MIN_LENGTH || !/[a-zA-Z]/.test(next) || !/\d/.test(next)) {
          details.push({ field: 'new_password', message: 'use ao menos 8 caracteres, com letras e números' })
        }
        failIfInvalid(details)
        record.password = next
        record.passwordChangedAt = now()
      })
    },
  }

  // ---- dashboard ----------------------------------------------------------

  const dashboard: DashboardService = {
    get(filters = {}) {
      return authorized(() => buildDashboard(filters.farmId, filters.periodDays ?? 14), 'dashboard.view')
    },
  }

  function buildDashboard(farmId: string | undefined, periodDays: number): Dashboard {
    const caller = currentViewer()
    if (farmId) {
      const farm = db.farms.find(item => item.id === farmId)
      const access = farm ? farmAccess(caller, farm) : 'hidden'
      if (access === 'hidden') fail('not_found', 'fazenda não encontrada')
      if (access === 'out_of_scope') fail('out_of_scope', 'fazenda fora do seu escopo')
    }
    const farms = db.farms.filter(item => canSeeFarm(caller, item) && (!farmId || item.id === farmId))
    const farmIds = new Set(farms.map(item => item.id))
    const fields = db.fields.filter(item => farmIds.has(item.farmId))
    const today = now()
    const isoDay = (offsetDays: number) => new Date(today.getTime() + offsetDays * DAY_MS).toISOString().slice(0, 10)
    const fieldLabel = (name: string) => name.replace(/^Talh[aã]o\s+/i, 'T')
    const suitable = fields.filter(item => item.sprayCondition === 'suitable')
    const companyIds = new Set(farms.map(item => item.companyId))
    const applicationsThisMonth = db.companies.filter(item => companyIds.has(item.id)).reduce((sum, item) => sum + item.applicationsThisMonth, 0)
    const recentlySprayed = fields.filter(item => item.lastApplicationAt && today.getTime() - item.lastApplicationAt.getTime() <= 30 * DAY_MS)
    // Planner data is not modeled yet: deterministic series shaped like the Figma chart.
    const hours = [6, 3, 0, 0, 7, 8, 5, 2, 6, 10, 4, 0, 0, 8, 9, 4, 6, 7, 5, 3, 2, 8, 6, 5, 4, 7, 6, 3, 5, 6, 7]
    const applications = [2, 0, 0, 0, 2, 2, 1, 0, 1, 3, 1, 0, 0, 1, 2, 1, 1, 2, 1, 0, 0, 2, 1, 1, 1, 2, 1, 0, 1, 1, 2]
    const days = Array.from({ length: periodDays }, (_, index) => ({
      date: isoDay(index - periodDays + 1),
      suitableHours: farms.length ? hours[index % hours.length]! : 0,
      applications: farms.length ? applications[index % applications.length]! : 0,
    }))
    const totalHours = days.reduce((sum, day) => sum + day.suitableHours, 0)
    const totalApplications = days.reduce((sum, day) => sum + day.applications, 0)
    const upcoming: UpcomingWindow[] = fields.slice(0, 5).map((field, index) => {
      const hasWindow = field.sprayCondition !== 'rain' && field.sprayCondition !== 'unsuitable'
      return {
        fieldId: field.id,
        fieldLabel: fieldLabel(field.name),
        farmId: field.farmId,
        date: isoDay(index < 3 ? 0 : index - 2),
        startTime: hasWindow ? (index % 2 ? '07:00' : '06:00') : null,
        endTime: hasWindow ? (index % 2 ? '09:00' : '09:30') : null,
        cropVariety: field.variety,
        operation: 'fungicida',
        condition: field.sprayCondition ?? 'attention',
      }
    })
    const hasData = farms.length > 0
    return {
      generatedAt: today,
      farmId: farmId ?? null,
      farmCount: farms.length,
      indicators: {
        suitableWindowsToday: {
          count: suitable.length,
          fieldLabels: suitable.slice(0, 3).map(item => fieldLabel(item.name)),
          nextStartAt: suitable.length ? new Date(`${isoDay(0)}T09:00:00Z`) : null,
        },
        applicationsThisMonth: { count: applicationsThisMonth, confirmedThisWeek: Math.min(applicationsThisMonth, 4) },
        sprayedArea: {
          areaHa: roundOneDecimal(recentlySprayed.reduce((sum, item) => sum + item.areaHa, 0)),
          productiveAreaHa: roundOneDecimal(fields.reduce((sum, item) => sum + item.areaHa, 0)),
        },
        pendingItems: hasData ? { count: 2, headline: '1 carência vence amanhã' } : { count: 0, headline: null },
      },
      windowsVsApplications: {
        days,
        summary: hasData
          ? `Nos últimos ${periodDays} dias houve ${totalHours} h de janela apta e ${totalApplications} aplicações. Quinta e sexta foram perdidas pela chuva.`
          : null,
      },
      upcomingWindows: upcoming,
      agentAlerts: hasData
        ? [
            { id: 'alert-phi', kind: 'pre_harvest_interval', tone: 'attention', title: 'Carência do T4 vence amanhã', description: 'Colheita prevista em 10 dias. Confirme que a última aplicação respeita os 30 dias.' },
            { id: 'alert-rain', kind: 'rain', tone: 'highlight', title: 'Chuva de 18 mm na quinta', description: 'Evite aplicações na quarta à tarde: o produto precisa de 2 h sem chuva.' },
            { id: 'alert-forecast', kind: 'forecast_update', tone: 'info', title: 'Previsão atualizada às 05:40', description: `Dados do Open-Meteo para os ${fields.length} talhões.` },
          ]
        : [],
      recentActivity: hasData
        ? [
            { id: 'activity-1', actor: { kind: 'user', name: 'Carlos Mendes' }, description: 'registrou aplicação no T3 · 38,6 ha', occurredAt: new Date(today.getTime() - 2 * 3_600_000) },
            { id: 'activity-2', actor: { kind: 'user', name: 'Marina Rezende' }, description: 'confirmou plano do agente para T7', occurredAt: new Date(today.getTime() - 19 * 3_600_000) },
            { id: 'activity-3', actor: { kind: 'user', name: 'Bruno Blauzius' }, description: 'cadastrou Helena Prado como proprietária', occurredAt: new Date(today.getTime() - 21 * 3_600_000) },
            { id: 'activity-4', actor: { kind: 'agent', name: 'Agente' }, description: 'sugeriu dividir T3 e T7 em dois dias', occurredAt: new Date(today.getTime() - 22 * 3_600_000) },
          ]
        : [],
    }
  }

  return { users, companies, profile, dashboard }
}
