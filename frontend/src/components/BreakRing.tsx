import type { BreakTimerState } from '../api'
import { formatMMSS } from '../utils/useBreakTimer'

export function BreakRing({
  state,
  remainingSec,
  totalSec,
  defaultWorkMinutes,
  phaseLabel,
  size = 148,
}: {
  state: BreakTimerState
  remainingSec: number
  totalSec: number
  defaultWorkMinutes: number
  phaseLabel: string
  size?: number
}) {
  const stroke = 8
  const r = (size - stroke) / 2
  const c = 2 * Math.PI * r

  let ratio = 0
  let displaySec = remainingSec
  if (state === 'break_done') {
    ratio = 1
    displaySec = 0
  } else if (state === 'idle' || state === 'outside_hours') {
    ratio = 0
    displaySec = defaultWorkMinutes * 60
  } else if (totalSec > 0) {
    ratio = Math.min(1, Math.max(0, remainingSec / totalSec))
  }

  const cls = state === 'working' ? ' working' : state === 'break_done' ? ' done' : ''

  return (
    <div className={'break-ring' + cls} style={{ width: size, height: size }}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden>
        <circle className="ring-bg" cx={size / 2} cy={size / 2} r={r} strokeWidth={stroke} />
        <circle
          className="ring-fg"
          cx={size / 2}
          cy={size / 2}
          r={r}
          strokeWidth={stroke}
          strokeDasharray={c}
          strokeDashoffset={c * (1 - ratio)}
        />
      </svg>
      <div className="break-ring-center">
        <div className="break-time num">{formatMMSS(displaySec)}</div>
        <div className="break-phase">{phaseLabel}</div>
      </div>
    </div>
  )
}
