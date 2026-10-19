import type { CSSProperties, ReactNode } from 'react'
import {
  Activity, CalendarCheck2, FileText, HeartPulse, Hospital, LayoutGrid, LineChart as LineIcon,
  Pill, PillBottle, Plug, Search, Stethoscope, TestTube2, type LucideIcon,
} from 'lucide-react'

// Each kind of record has one icon and one identity color (--c-<kind>),
// used wherever that kind appears: nav, lists, drawers, the timeline.
export const KIND: Record<string, { icon: LucideIcon; color: string }> = {
  overview: { icon: LayoutGrid, color: 'var(--accent)' },
  checkins: { icon: CalendarCheck2, color: 'var(--c-checkins)' },
  habits: { icon: CalendarCheck2, color: 'var(--c-habits)' },
  conditions: { icon: HeartPulse, color: 'var(--c-conditions)' },
  symptoms: { icon: Activity, color: 'var(--c-symptoms)' },
  visits: { icon: Hospital, color: 'var(--c-visits)' },
  medications: { icon: Pill, color: 'var(--c-medications)' },
  doses: { icon: PillBottle, color: 'var(--c-medications)' },
  reports: { icon: FileText, color: 'var(--c-reports)' },
  lab_results: { icon: TestTube2, color: 'var(--c-lab_results)' },
  metrics: { icon: LineIcon, color: 'var(--c-metrics)' },
  connect: { icon: Plug, color: 'var(--c-connect)' },
  search: { icon: Search, color: 'var(--ink-2)' },
  stethoscope: { icon: Stethoscope, color: 'var(--accent)' },
}

export const kindStyle = (kind: string) => ({ '--k': KIND[kind]?.color } as CSSProperties)

export function KindIcon({ kind, size = 'md' }: { kind: string; size?: 'sm' | 'md' | 'lg' }) {
  const k = KIND[kind] || KIND.search
  const Icon = k.icon
  const px = size === 'sm' ? 13 : size === 'lg' ? 20 : 16
  return (
    <span className={'chip-ico' + (size === 'md' ? '' : ' ' + size)} style={kindStyle(kind)}>
      <Icon size={px} strokeWidth={2.2} />
    </span>
  )
}

export function IconButton({ label, onClick, children, className = '', small }: {
  label: string; onClick: () => void; children: ReactNode; className?: string; small?: boolean
}) {
  return (
    <button type="button" className={'icon-btn ' + (small ? 'sm ' : '') + className} onClick={onClick} aria-label={label} title={label}>
      {children}
    </button>
  )
}

export function Switch({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: string }) {
  return (
    <label className="switch" aria-label={label}>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span />
    </label>
  )
}

export function Empty({ icon: Icon, children, compact }: { icon?: LucideIcon; children: ReactNode; compact?: boolean }) {
  return (
    <div className={'empty' + (compact ? ' compact' : '')}>
      {Icon && !compact && <Icon size={28} strokeWidth={1.6} />}
      <div>{children}</div>
    </div>
  )
}

// Where a value sits against its reference range. The scale reaches a bit
// past the range on both sides and stretches to include the value, so a
// far-out value still shows how far out it is.
export function RangeBar({ value, low, high, flag }: { value?: number | null; low?: number | null; high?: number | null; flag?: string | null }) {
  if (value == null || (low == null && high == null)) return null
  let lo = low ?? 0
  let hi = high ?? (low! * 2 || 1)
  const span = hi - lo || Math.abs(hi) || 1
  let min = Math.min(lo - span * 0.35, value)
  let max = Math.max(hi + span * 0.35, value)
  if (low != null && low >= 0) min = Math.max(min, Math.min(0, value))
  const pad = (max - min) * 0.04
  min -= pad
  max += pad
  const pct = (v: number) => ((v - min) / (max - min)) * 100
  if (low == null) lo = min
  if (high == null) hi = max
  return (
    <div className="range" aria-hidden>
      <div className="track" />
      <div className="band" style={{ left: pct(lo) + '%', width: pct(hi) - pct(lo) + '%' }} />
      <div className={'mark' + (flag === '偏高' ? ' high' : flag === '偏低' ? ' low' : '')} style={{ left: pct(value) + '%' }} />
    </div>
  )
}

// A tiny trend line for a tile: no axes, last point marked.
export function Sparkline({ values }: { values: number[] }) {
  if (values.length < 2) return <svg className="spark" />
  const W = 120, H = 28, P = 3
  const lo = Math.min(...values), hi = Math.max(...values)
  const y = (v: number) => (hi === lo ? H / 2 : P + (1 - (v - lo) / (hi - lo)) * (H - 2 * P))
  const x = (i: number) => P + (i / (values.length - 1)) * (W - 2 * P)
  const d = values.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('')
  return (
    <svg className="spark" viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" aria-hidden>
      <path d={d} vectorEffect="non-scaling-stroke" />
      <circle cx={x(values.length - 1)} cy={y(values[values.length - 1])} r={2.5} />
    </svg>
  )
}

export function Ring({ value, total, size = 40 }: { value: number; total: number; size?: number }) {
  const r = (size - 6) / 2
  const c = 2 * Math.PI * r
  const f = total ? value / total : 0
  return (
    <svg className="ring" width={size} height={size} aria-hidden>
      <circle className="bg" cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={5} />
      <circle className="fg" cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={5}
        strokeDasharray={c} strokeDashoffset={c * (1 - f)} />
    </svg>
  )
}

export const HABIT_EMOJI = ['💊', '🏃', '🚶', '😴', '💧', '🥗', '🍎', '🧘', '🏋️', '🚴', '🦷', '📖', '☀️', '🚭', '🍵', '🩺', '🩸', '⚖️']

export function EmojiPicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div className="emoji-grid" role="radiogroup">
      {HABIT_EMOJI.map((e) => (
        <button type="button" key={e} className={value === e ? 'on' : ''} onClick={() => onChange(value === e ? '' : e)} aria-label={e}>
          {e}
        </button>
      ))}
    </div>
  )
}
