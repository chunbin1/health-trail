import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { api, type Entity, type Rec, type Schema } from './api'
import { DialogHost } from './components/Dialog'
import { t, tx, type Lang } from './i18n'

interface Ctx {
  schema: Schema
  entity: (name: string) => Entity
  version: number // bumped after every write; views reload on it
  changed: () => void
  toast: (msg: string) => void
  lang: Lang
  switchLang: (l: Lang) => void
}

const AppCtx = createContext<Ctx>(null!)
export const useApp = () => useContext(AppCtx)

export function AppProvider({ schema, lang, switchLang, children }: { schema: Schema; lang: Lang; switchLang: (l: Lang) => void; children: ReactNode }) {
  const [version, setVersion] = useState(0)
  const [msg, setMsg] = useState('')
  const entity = useCallback((name: string) => schema.entities.find((e) => e.name === name)!, [schema])
  const changed = useCallback(() => setVersion((v) => v + 1), [])
  const toast = useCallback((m: string) => {
    setMsg(m)
    window.setTimeout(() => setMsg((cur) => (cur === m ? '' : cur)), 2200)
  }, [])
  return (
    <AppCtx.Provider value={{ schema, entity, version, changed, toast, lang, switchLang }}>
      {children}
      {msg && <div className="toast">{msg}</div>}
      <DialogHost />
    </AppCtx.Provider>
  )
}

// ---- routing: #/page or #/page/entity/id (the id opens a record drawer)

export interface Route {
  page: string
  open?: { entity: string; id: number }
}

function parse(): Route {
  const parts = location.hash.replace(/^#\/?/, '').split('?')[0].split('/').filter(Boolean)
  const page = parts[0] || 'overview'
  if (parts.length >= 3 && /^\d+$/.test(parts[2])) return { page, open: { entity: parts[1], id: Number(parts[2]) } }
  return { page }
}

export function useRoute(): [Route, (r: Route) => void] {
  const [route, setRoute] = useState(parse)
  useEffect(() => {
    const on = () => setRoute(parse())
    window.addEventListener('hashchange', on)
    return () => window.removeEventListener('hashchange', on)
  }, [])
  const go = useCallback((r: Route) => {
    location.hash = '/' + r.page + (r.open ? `/${r.open.entity}/${r.open.id}` : '')
  }, [])
  return [route, go]
}

const currentPage = () => location.hash.replace(/^#\/?/, '').split(/[/?]/)[0] || 'overview'

// hashParam reads ?k=v from the hash route (#/metrics?type=体重).
export function hashParam(k: string) {
  return new URLSearchParams(location.hash.split('?')[1] || '').get(k) || ''
}

export function openRecord(entity: string, id: number) {
  const page = currentPage()
  location.hash = `/${page}/${entity}/${id}`
}

export function closeRecord() {
  const page = currentPage()
  location.hash = '/' + page
}

// ---- data loading

export function useLoad<T>(fn: () => Promise<T>, deps: unknown[]): { data: T | undefined; error: string; loading: boolean; reload: () => void } {
  const { version } = useApp()
  const [data, setData] = useState<T>()
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    let live = true
    setLoading(true)
    fn()
      .then((d) => live && (setData(d), setError('')))
      .catch((e) => live && setError(String(e.message || e)))
      .finally(() => live && setLoading(false))
    return () => {
      live = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, version, tick])
  return { data, error, loading, reload: () => setTick((t) => t + 1) }
}

// The body metrics to choose from: the built-in ones, then the ones the user
// added (metric_kinds), with each one's unit. `loaded` is false until the
// added ones have arrived.
export function useMetricKinds() {
  const { schema } = useApp()
  const { data } = useLoad(() => api.list('metric_kinds', { limit: 1000, asc: true }), [])
  return useMemo(() => {
    const kinds = data?.rows || []
    return {
      loaded: !!data,
      kinds,
      // BMI next to the height it's worked out with
      types: [...new Set([...schema.metric_types.flatMap((m) => (m === '身高' ? [m, ...schema.metric_derived] : [m])), ...kinds.map((k) => k.name as string)])],
      derived: schema.metric_derived,
      units: { ...schema.metric_units, ...Object.fromEntries(kinds.map((k) => [k.name, k.unit || ''])) } as Record<string, string>,
    }
  }, [schema, data])
}

// Records a ref field may point at, for selects and for showing names.
const refCache = new Map<string, Promise<Rec[]>>()
export function useRefOptions(entity: string | undefined) {
  const { version } = useApp()
  const [rows, setRows] = useState<Rec[]>([])
  useEffect(() => {
    if (!entity) return
    const key = entity + ':' + version
    if (!refCache.has(key)) refCache.set(key, api.list(entity, { limit: 1000 }).then((r) => r.rows))
    let live = true
    refCache.get(key)!.then((r) => live && setRows(r))
    return () => {
      live = false
    }
  }, [entity, version])
  return rows
}

export function recTitle(e: Entity, r: Rec): string {
  if (e.name === 'visits') return [r.diagnosis, r.department, r.hospital].filter(Boolean).join(' · ') || tx(r.visit_type) || t('就诊|quick')
  if (e.name === 'metrics') return tx(r.metric_type)
  return String(r[e.titleKey] ?? `#${r.id}`)
}
