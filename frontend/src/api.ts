import { isEN, t } from './i18n'

// The schema comes from the server (internal/store/schema.go): the forms
// and lists here are drawn from it rather than declared twice.

export type FieldType = 'text' | 'longtext' | 'int' | 'real' | 'date' | 'datetime' | 'time' | 'times' | 'enum' | 'ref' | 'bool'

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
  metric_derived: string[] // worked out from others (BMI), not recorded
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
  upcoming_appointments: Rec[]
  meds_today: MedDay[]
  recent_symptoms: Rec[]
  abnormal_labs_latest: Rec[]
  latest_metrics: Rec[]
  today_checkins: HabitDay[]
  counts: Record<string, number>
}

// A medicine with reminder times, and whether each of today's times is taken.
export interface MedDay {
  medication: Rec
  slots: { time: string; dose: Rec | null }[]
}

// Appointments written to Calendar and medicines to Reminders (Mac app only).
export interface CalSync {
  events: boolean
  reminders: boolean
  title: string
  source: string
  events_access: 'not_asked' | 'full' | 'denied' | 'write_only'
  reminders_access: 'not_asked' | 'full' | 'denied' | 'write_only'
  last_at: string
  last_error: string
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

// ListChoice is a list put in order: any item not in order comes after.
export interface ListChoice {
  order: string[]
  hidden: string[]
}

export interface Info {
  data_dir: string
  version: string
  lan: boolean
  lang: string
  lists: Record<string, ListChoice> // lists the person arranged, by name (useListChoice)
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

// Folders the sandboxed App Store build may read, beyond Downloads.
export interface Folders {
  sandboxed: boolean
  folders: string[]
}

// A newer release and how installing it goes, in the Mac app only.
export interface UpdateRelease {
  version: string
  name: string
  notes: string
  page: string
  published: string
}

export interface UpdateStatus {
  current: string
  auto: boolean
  state: {
    checked_at?: string
    checking: boolean
    latest?: UpdateRelease // only when newer than this build
    error?: string // of the last check
    install_error?: string // why the last install didn't finish
    phase?: 'backup' | 'download' | 'restart'
    done?: number
    total?: number
  }
  installable?: boolean // the release has the zip the app installs
  why_not?: string // why this copy can't replace itself
}

export class ApiError extends Error {}

// In-app assistant: it reads the archive with the MCP tools; what it would
// write comes back as cards, saved only after confirm (or by automatic saving).
export interface AssistantConfig {
  configured: boolean
  base_url: string
  model: string
  has_key: boolean
  speech: boolean
  auto_save: boolean
  providers: AIProvider[]
}

// A service AI助手 can be set up with in a few clicks (assistant.Providers).
export interface AIProvider {
  id: string
  name: string
  name_en: string
  base_url: string
  models: { id: string; note: string; note_en: string }[] // the first is the default
  key_page: string
  signup: string
  invite: boolean // signup carries the author's invite code
}

export interface LabDraft {
  item_name?: string
  value?: string | number
  unit?: string
  ref_range?: string
  flag?: string
  panel?: string
  test_date?: string
  notes?: string
}

export interface ChatHead {
  id: string
  updated: string
  title: string
  tokens?: number
}

export interface ChatMsgWire {
  id: string
  role: 'user' | 'assistant'
  text: string
  images?: { mime: string; data: string }[]
  imageCount?: number
  items?: DraftItem[] | null
  unclear?: string
  reply?: string
  usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number }
  saved?: Record<number, { type?: string; id?: number }>
  locked?: boolean
  steps?: ToolStep[]
}

// One reply in the assistant's log (assistant.log), newest first.
export interface AssistantLogEntry {
  at: string
  model: string
  ask: string
  requests: number
  tokens: number
  failed: boolean
  text: string
}

export interface AssistantLogView {
  entries: AssistantLogEntry[]
  cut: boolean
  size: number
}

// One tool the assistant called while answering.
export interface ToolStep {
  tool: string
  type?: string
  id?: number
  text?: string
  error?: boolean
}

export interface DraftItem {
  kind: 'record' | 'checkin' | 'lab_report' | 'update' | string
  type?: string
  fields?: Record<string, any>
  condition_name?: string
  medication_name?: string
  habit?: string
  date?: string
  value?: number | string | null
  notes?: string
  report?: Record<string, any>
  panel?: string
  results?: LabDraft[]
  summary?: string
  // a check-in card: the habit's unit
  unit?: string
  problem?: string
  // kind 'update': record id of type; fields are the new values, before the old
  id?: number
  before?: Record<string, any>
}

export interface TokenUse {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  cached_tokens?: number
}

export interface Draft {
  items: DraftItem[]
  unclear?: string
  reply?: string
  steps?: ToolStep[]
  usage?: TokenUse
}

// A draft keeps running after the page is left. state "pending" means the
// reply is not back yet.
export interface DraftStatus {
  state: 'none' | 'pending' | 'done' | 'error'
  id?: string
  text?: string
  items?: DraftItem[]
  unclear?: string
  reply?: string
  steps?: ToolStep[]
  usage?: TokenUse
  error?: string
}

export interface ChatTurn {
  role: 'user' | 'assistant'
  text: string
  images?: { mime: string; data: string }[]
}

export interface CommitResult {
  ok: boolean
  error?: string
  saved: { index: number; kind: string; type?: string; id?: number; title?: string }[]
  failed_index?: number
}

export interface SpeechState {
  available: boolean
  listening: boolean
  text: string
  error?: string
}

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
  // only in the Mac app's window
  autostart: () => req<{ enabled: boolean }>('GET', '/api/autostart'),
  notifications: () => req<{ allowed: boolean | null }>('GET', '/api/notifications'),
  calsync: () => req<CalSync>('GET', '/api/calsync'),
  setCalsync: (body: { events?: boolean; reminders?: boolean }) => req<CalSync>('PUT', '/api/calsync', body),
  openCalPrivacy: (reminders: boolean) => req<{ ok: boolean }>('POST', '/api/calsync/privacy' + (reminders ? '?reminders=1' : '')),
  openNotificationSettings: () => req<{ ok: boolean }>('POST', '/api/notifications/settings'),
  setAutostart: (enabled: boolean) => req<{ enabled: boolean }>('PUT', '/api/autostart', { enabled }),
  folders: () => req<Folders>('GET', '/api/folders'),
  addFolder: () => req<Folders>('POST', '/api/folders'),
  removeFolder: (path: string) => req<Folders>('DELETE', '/api/folders', { path }),
  updateStatus: () => req<UpdateStatus>('GET', '/api/update'),
  checkUpdate: () => req<UpdateStatus>('POST', '/api/update/check'),
  setAutoUpdate: (auto: boolean) => req<UpdateStatus>('PUT', '/api/update', { auto }),
  installUpdate: () => req<UpdateStatus>('POST', '/api/update/install'),
  openReleasePage: () => req<{ ok: boolean }>('POST', '/api/update/page'),
  setLang: (lang: string) => req<Info>('PUT', '/api/settings', { lang }),
  setList: (name: string, choice: ListChoice) => req<Info>('PUT', '/api/settings', { lists: { [name]: choice } }),
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
  metricSeries: (type: string, from?: string, to?: string) => req<Rec[]>('GET', '/api/metrics/series' + qs({ type, from, to })),
  checkinDay: (date?: string) => req<{ date: string; habits: HabitDay[] }>('GET', '/api/checkins/day' + qs({ date })),
  checkinStats: (from?: string, to?: string, habit_id?: number) => req<HabitStats[]>('GET', '/api/checkins/stats' + qs({ from, to, habit_id })),
  checkin: (habit_id: number, date: string, value?: number | null, notes?: string) =>
    req<Rec>('POST', '/api/checkins', { habit_id, date, value: value ?? null, notes: notes ?? '' }),
  uncheck: (habit_id: number, date: string) => req<unknown>('DELETE', '/api/checkins' + qs({ habit_id, date })),
  // JSON, not a file upload: the Mac window's address scheme drops a
  // multipart file body and the server then sees an empty request.
  uploadBase64: (owner_type: string, owner_id: number, filename: string, mime: string, data: string) =>
    req<Attachment>('POST', '/api/attachments', { owner_type, owner_id, filename, mime, data }),
  upload: async (owner_type: string, owner_id: number, file: File) => {
    const data = await new Promise<string>((resolve, reject) => {
      const r = new FileReader()
      r.onload = () => {
        const s = String(r.result || '')
        const i = s.indexOf(',')
        resolve(i >= 0 ? s.slice(i + 1) : s)
      }
      r.onerror = () => reject(new ApiError(t('这张图片读不出来')))
      r.readAsDataURL(file)
    })
    return api.uploadBase64(owner_type, owner_id, file.name || '附件', file.type, data)
  },
  removeAttachment: (id: number) => req<unknown>('DELETE', `/api/attachments/${id}`),
  // the Mac window only: opens the file in its Mac app (or the Finder)
  openAttachment: (id: number) => req<{ revealed: boolean }>('POST', `/api/attachments/${id}/open`),
  assistant: () => req<AssistantConfig>('GET', '/api/assistant'),
  setAssistant: (body: { base_url?: string; api_key?: string; model?: string; clear_key?: boolean; auto_save?: boolean }) =>
    req<AssistantConfig>('PUT', '/api/assistant', body).then((c) => { window.dispatchEvent(new Event('assistant-config')); return c }),
  checkAssistant: () => req<{ ok: boolean }>('POST', '/api/assistant/check'),
  openProviderPage: (id: string, page: 'signup' | 'key') =>
    req<{ ok: boolean }>('POST', '/api/assistant/providers/' + encodeURIComponent(id) + '/open?page=' + page),
  assistantDraft: (body: { id: string; messages: ChatTurn[] }) => req<Draft>('POST', '/api/assistant/draft', body),
  assistantDraftStatus: () => req<DraftStatus>('GET', '/api/assistant/draft'),
  assistantCommit: (items: DraftItem[]) => req<CommitResult>('POST', '/api/assistant/commit', { items }),
  assistantLog: () => req<AssistantLogView>('GET', '/api/assistant/log'),
  assistantChats: () => req<ChatHead[]>('GET', '/api/assistant/chats'),
  assistantChat: (id: string) => req<ChatHead & { messages: ChatMsgWire[] }>('GET', '/api/assistant/chats/' + encodeURIComponent(id)),
  saveAssistantChat: (body: { id: string; messages: ChatMsgWire[] }) => req<ChatHead>('PUT', '/api/assistant/chats', body),
  deleteAssistantChat: (id: string) => req<{ ok: boolean }>('DELETE', '/api/assistant/chats/' + encodeURIComponent(id)),
  speechStart: () => req<SpeechState>('POST', '/api/assistant/speech/start'),
  speechStop: () => req<SpeechState>('POST', '/api/assistant/speech/stop'),
  speech: () => req<SpeechState>('GET', '/api/assistant/speech'),
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

export function labValue(r: Rec) {
  return r.value_text || fmtNum(r.value_num)
}

export function metricValue(r: Rec) {
  const v = fmtNum(r.value)
  return r.value2 != null ? `${v}/${fmtNum(r.value2)}` : v
}
