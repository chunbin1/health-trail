import { useLayoutEffect, useRef, useState } from 'react'
import { fmtDate, fmtNum } from '../api'
import { t, tx } from '../i18n'
import { Empty } from '../ui'

export interface Point {
  date: string // YYYY-MM-DD[ HH:MM]
  v: number
  v2?: number | null
  flag?: string | null
}

// A time-series chart: points placed by date, the reference range as a
// band, out-of-range points in the status color. A second series
// (diastolic pressure) is drawn when points carry v2; then both lines are
// labelled at their ends and a legend sits above. Hover shows a crosshair
// and the nearest reading.
export function LineChart({ points, low, high, unit, height = 240, names }: {
  points: Point[]
  low?: number | null
  high?: number | null
  unit?: string | null
  height?: number
  names?: [string, string]
}) {
  const [hover, setHover] = useState<number | null>(null)
  // drawn at the box's real width, so labels stay 11px however narrow it is
  const box = useRef<HTMLDivElement>(null)
  const [W, setW] = useState(640)
  // measured before the first paint, so a phone never sees a frame at 640px
  useLayoutEffect(() => {
    if (!box.current) return
    setW(Math.max(240, box.current.clientWidth))
    const ro = new ResizeObserver(([e]) => setW(Math.max(240, Math.round(e.contentRect.width))))
    ro.observe(box.current)
    return () => ro.disconnect()
  }, [points.length === 0])
  if (points.length === 0) return <Empty>{t('还没有数据')}</Empty>
  const two = points.some((p) => p.v2 != null)
  const H = height, L = 40, R = two ? 54 : 14, T = 14, B = 26
  const ms = (d: string) => new Date(d.replace(' ', 'T')).getTime()
  const ts = points.map((p) => ms(p.date))
  let t0 = Math.min(...ts), t1 = Math.max(...ts)
  if (t0 === t1) { t0 -= 864e5; t1 += 864e5 }
  const vals = points.flatMap((p) => (p.v2 != null ? [p.v, p.v2] : [p.v]))
  if (low != null) vals.push(low)
  if (high != null) vals.push(high)
  let y0 = Math.min(...vals), y1 = Math.max(...vals)
  const pad = (y1 - y0) * 0.12 || Math.abs(y1) * 0.1 || 1
  y0 -= pad; y1 += pad
  const x = (v: number) => L + ((v - t0) / (t1 - t0)) * (W - L - R)
  const y = (v: number) => T + (1 - (v - y0) / (y1 - y0)) * (H - T - B)
  const ticks = niceTicks(y0, y1, 4)
  const path = (key: 'v' | 'v2') =>
    points.filter((p) => p[key] != null).map((p, i) => `${i ? 'L' : 'M'}${x(ms(p.date)).toFixed(1)},${y(p[key] as number).toFixed(1)}`).join('')
  const xLabels = [0, Math.floor((points.length - 1) / 2), points.length - 1].filter((v, i, a) => a.indexOf(v) === i)
  const h = hover != null ? points[hover] : null
  const last = points[points.length - 1]

  function move(e: React.MouseEvent<SVGSVGElement>) {
    const px = e.clientX - e.currentTarget.getBoundingClientRect().left
    let best = 0
    ts.forEach((tv, i) => { if (Math.abs(x(tv) - px) < Math.abs(x(ts[best]) - px)) best = i })
    setHover(best)
  }

  return (
    <div ref={box} style={{ position: 'relative' }}>
      {two && names && (
        <div className="legend" style={{ marginBottom: 6 }}>
          <span><i style={{ background: 'var(--accent)' }} />{names[0]}</span>
          <span><i style={{ background: 'var(--series-2)' }} />{names[1]}</span>
        </div>
      )}
      <svg className="chart" width={W} height={H} onMouseMove={move} onMouseLeave={() => setHover(null)} role="img"
        aria-label={t('{n} 个读数，最近 {value} {unit}', { n: points.length, value: fmtNum(last.v) + (last.v2 != null ? '/' + fmtNum(last.v2) : ''), unit: unit || '' })}>
        {(low != null || high != null) && (
          <>
            <rect className="band" x={L} width={W - L - R} y={y(high ?? y1)} height={Math.max(0, y(low ?? y0) - y(high ?? y1))} />
            {high != null && <line className="band-edge" x1={L} x2={W - R} y1={y(high)} y2={y(high)} />}
            {low != null && <line className="band-edge" x1={L} x2={W - R} y1={y(low)} y2={y(low)} />}
          </>
        )}
        {ticks.map((v) => (
          <g key={v}>
            <line className="grid-line" x1={L} x2={W - R} y1={y(v)} y2={y(v)} />
            <text x={L - 8} y={y(v) + 4} textAnchor="end">{fmtNum(v)}</text>
          </g>
        ))}
        {xLabels.map((i) => (
          <text key={i} x={x(ts[i])} y={H - 6} textAnchor={i === 0 ? 'start' : i === points.length - 1 ? 'end' : 'middle'}>
            {fmtDate(points[i].date.slice(0, 10))}
          </text>
        ))}
        {h && <line className="cross" x1={x(ts[hover!])} x2={x(ts[hover!])} y1={T} y2={H - B} />}
        <path className="line" d={path('v')} />
        {two && <path className="line2" d={path('v2')} />}
        {points.map((p, i) => (
          <g key={i}>
            <circle className={'pt' + (p.flag === '偏高' ? ' high' : p.flag === '偏低' ? ' low' : '')} cx={x(ts[i])} cy={y(p.v)} r={hover === i ? 5.5 : 4} />
            {p.v2 != null && <circle className="pt2" cx={x(ts[i])} cy={y(p.v2)} r={hover === i ? 5.5 : 4} />}
          </g>
        ))}
        {two && names && (
          <>
            <text className="lbl1" x={x(ts[ts.length - 1]) + 8} y={y(last.v) + 4}>{names[0]}</text>
            {last.v2 != null && <text className="lbl2" x={x(ts[ts.length - 1]) + 8} y={y(last.v2) + 4}>{names[1]}</text>}
          </>
        )}
      </svg>
      {h && (
        <div className="chart-tip" style={{ left: Math.min(Math.max(x(ts[hover!]), 70), W - 70) }}>
          <div className="d">{fmtDate(h.date)}</div>
          <div><b className="num">{fmtNum(h.v)}{h.v2 != null ? ' / ' + fmtNum(h.v2) : ''}</b> {unit}
            {h.flag && h.flag !== '正常' && <span className={h.flag === '偏高' ? 'flag-high' : 'flag-low'}> · {tx(h.flag)}</span>}
          </div>
        </div>
      )}
    </div>
  )
}

function niceTicks(lo: number, hi: number, n: number) {
  const span = hi - lo
  const step0 = span / n
  const mag = Math.pow(10, Math.floor(Math.log10(step0)))
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= step0) || step0
  const out: number[] = []
  for (let v = Math.ceil(lo / step) * step; v <= hi + 1e-9; v += step) out.push(Math.round(v * 1e6) / 1e6)
  return out
}
