import { isEN, t } from './i18n'

// The schema comes from the server (internal/store/schema.go): the forms
// and lists here are drawn from it rather than declared twice.

export type FieldType = 'text' | 'longtext' | 'int' | 'real' | 'date' | 'datetime' | 'time' | 'enum' | 'ref' | 'bool'

export interface Field {
  name: string
  label: string
  type: FieldType
  required?: boolean
  options?: string[]
  custom?: string // enum: the names of this entity's rows are choices too
  ref?: string
  unit?: string
  hint?: string
  list?: boolean
}

export interface Entity {
  name: string
  label: string
  dateKey: string
  titleKey: string
  fields: Field[]
}

export interface Schema {
  entities: Entity[]
  metric_types: string[]
  metric_units: Record<string, string>
  // what the English UI shows for the Chinese above (schema_en.go)
  en: { text: Record<string, string>; nouns: Record<string, string> }
}

export type Rec = Record<string, any> & { id: number }

export interface Attachment {
  id: number
  filename: string
  mime: string
  size: number
  created_at: string
}

export interface HabitDay {
  habit: Rec
  checkin: Rec | null
}

export interface Overview {
  today: string
  active_conditions: Rec[]
  current_medications: Rec[]
  recent_visits: Rec[]
  upcoming_follow_ups: Rec[]
  recent_symptoms: Rec[]
  abnormal_labs_latest: Rec[]
  latest_metrics: Rec[]
  today_checkins: HabitDay[]
  counts: Record<string, number>
}

export interface LabItem {
  item_name: string
  count: number
  latest_date: string
  latest_value: number | null
  unit: string | null
  latest_flag: string | null
  panel: string | null
}

export interface LabRow {
  item_name: string
  value_num: number | null
  value_text?: string
  flag?: string
  unit?: string
  ref_range?: string
}

export interface HabitStats {
  habit: Rec
  days_done: number
  days_in_range: number
  completion_rate: number
  current_streak: number
  longest_streak_in_range: number
  total_value?: number
  dates: string[]
}

export interface Info {
  data_dir: string
  version: string
  lan: boolean
  lang: string
  listening: string
  listen_error: string
  phone_urls: string[] | null
  mcp_stdio_command: string
  claude_desktop_config: string
  claude_code_command: string
  mcp_http_url: string
  claude_code_http: string
  codex_command: string
  codex_config: string
  codex_http: string
  mcp_args: string
}

export interface BackupConfig {
  dir: string
  keep: number
  daily: boolean
  daily_at: string
  weekly: boolean
  weekday: number
  weekly_at: string
  on_change: boolean
  on_quit: boolean
}

export interface BackupEntry {
  name: string
  size: number
  time: string
}

export interface BackupState {
  config: BackupConfig
  status: { last_at?: string; last_file?: string; last_error?: string; error_at?: string }
  backups: BackupEntry[]
  suggested_dir: string
  can_choose_dir: boolean
}

export class ApiError extends Error {}

async function req<T>(method: string, url: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {} }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    init.body = JSON.stringify(body)
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
  }
  const res = await fetch(url, init)
  const text = await res.text()
  let data: any = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    throw new ApiError(t('服务器返回了无法解析的内容（{status}）', { status: res.status }))
  }
  if (!res.ok) throw new ApiError(data?.error || t('请求失败（{status}）', { status: res.status }))
  return data as T
}

const qs = (p: Record<string, string | number | boolean | undefined | null>) => {
  const u = new URLSearchParams()
  for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== null && v !== '') u.set(k, String(v))
  const s = u.toString()
  return s ? '?' + s : ''
}

export const api = {
  schema: () => req<Schema>('GET', '/api/schema'),
  info: () => req<Info>('GET', '/api/info'),
  setLAN: (lan: boolean) => req<Info>('PUT', '/api/settings', { lan }),
  setLang: (lang: string) => req<Info>('PUT', '/api/settings', { lang }),
  backup: () => req<BackupState>('GET', '/api/backup'),
  saveBackup: (c: BackupConfig) => req<BackupConfig>('PUT', '/api/backup', c),
  runBackup: () => req<{ name: string }>('POST', '/api/backup/run'),
  restoreBackup: (name: string) => req<{ ok: boolean }>('POST', '/api/backup/restore', { name }),
  chooseBackupDir: () => req<{ dir: string }>('POST', '/api/backup/choose-dir'),
  overview: () => req<Overview>('GET', '/api/overview'),
  timeline: (limit = 20) => req<{ type: string; id: number; date: string; title: string }[]>('GET', '/api/timeline' + qs({ limit })),
  search: (q: string) => req<{ type: string; type_label: string; rows: Rec[]; total: number }[]>('GET', '/api/search' + qs({ q })),
  list: (entity: string, p: Record<string, any> = {}) => req<{ rows: Rec[]; total: number }>('GET', `/api/r/${entity}` + qs(p)),
  get: (entity: string, id: number) => req<Record<string, any> & { record: Rec; attachments: Attachment[] }>('GET', `/api/r/${entity}/${id}`),
  create: (entity: string, data: Record<string, any>) => req<Rec>('POST', `/api/r/${entity}`, data),
  update: (entity: string, id: number, data: Record<string, any>) => req<Rec>('PUT', `/api/r/${entity}/${id}`, data),
  remove: (entity: string, id: number) => req<unknown>('DELETE', `/api/r/${entity}/${id}`),
  labItems: () => req<LabItem[]>('GET', '/api/labs/items'),
  labTrend: (name: string) => req<Rec[]>('GET', '/api/labs/trend' + qs({ name })),
  labParse: (text: string) => req<LabRow[]>('POST', '/api/labs/parse', { text }),
  labImport: (body: { report_id?: number; report?: Record<string, any>; panel?: string; rows: Record<string, any>[] }) =>
    req<{ report: Rec; lab_results: Rec[] }>('POST', '/api/labs/import', body),
  metricSeries: (type: string, from?: string, to?: string) => req<Rec[]>('GET', '/api/metrics/series' + qs({ type, from, to })),
  checkinDay: (date?: string) => req<{ date: string; habits: HabitDay[] }>('GET', '/api/checkins/day' + qs({ date })),
  checkinStats: (from?: string, to?: string, habit_id?: number) => req<HabitStats[]>('GET', '/api/checkins/stats' + qs({ from, to, habit_id })),
  checkin: (habit_id: number, date: string, value?: number | null, notes?: string) =>
    req<Rec>('POST', '/api/checkins', { habit_id, date, value: value ?? null, notes: notes ?? '' }),
  uncheck: (habit_id: number, date: string) => req<unknown>('DELETE', '/api/checkins' + qs({ habit_id, date })),
  upload: (owner_type: string, owner_id: number, file: File) => {
    const fd = new FormData()
    fd.set('owner_type', owner_type)
    fd.set('owner_id', String(owner_id))
    fd.set('file', file)
    return req<Attachment>('POST', '/api/attachments', fd)
  },
  removeAttachment: (id: number) => req<unknown>('DELETE', `/api/attachments/${id}`),
}

// ---- formatting helpers

export const pad = (n: number) => String(n).padStart(2, '0')
export const today = () => {
  const d = new Date()
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
export const nowLocal = () => {
  const d = new Date()
  return `${today()} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
export const addDays = (date: string, n: number) => {
  const [y, m, d] = date.split('-').map(Number)
  const t = new Date(y, m - 1, d + n)
  return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())}`
}

export function fmtDate(s?: string | null): string {
  if (!s) return ''
  const [d, time] = s.split(' ')
  const [y, m, day] = d.split('-')
  const thisYear = String(new Date().getFullYear())
  let date: string
  if (isEN()) date = `${MONTHS[Number(m) - 1]?.slice(0, 3)} ${Number(day)}` + (y === thisYear ? '' : `, ${y}`)
  else date = y === thisYear ? `${Number(m)}月${Number(day)}日` : `${y}年${Number(m)}月${Number(day)}日`
  return time ? `${date} ${time}` : date
}

export const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']

// fmtMonth names a month: 2026年10月, October 2026.
export function fmtMonth(y: string, m: string) {
  return isEN() ? `${MONTHS[Number(m) - 1]} ${y}` : `${y}年${Number(m)}月`
}

export function fmtNum(v: any): string {
  if (v === null || v === undefined || v === '') return ''
  if (typeof v === 'number') return String(Math.round(v * 1000) / 1000)
  return String(v)
}

export function flagClass(flag?: string | null) {
  if (flag === '偏高') return 'flag-high'
  if (flag === '偏低') return 'flag-low'
  if (flag === '异常') return 'flag-abn'
  return ''
}

export function flagArrow(flag?: string | null) {
  if (flag === '偏高') return '↑'
  if (flag === '偏低') return '↓'
  if (flag === '异常') return '!'
  return ''
}

export function labValue(r: Rec | LabRow) {
  return r.value_text || fmtNum(r.value_num)
}

export function metricValue(r: Rec) {
  const v = fmtNum(r.value)
  return r.value2 != null ? `${v}/${fmtNum(r.value2)}` : v
}
