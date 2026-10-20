import { useState, type FormEvent } from 'react'
import { Plus } from 'lucide-react'
import { addDays, api, fmtDate, fmtNum, metricValue, nowLocal, today } from '../api'
import { LineChart } from '../components/LineChart'
import { paren, t, tx } from '../i18n'
import { hashParam, openRecord, useApp, useLoad } from '../state'
import { Empty } from '../ui'

const RANGES = [30, 90, 365, 0]
const rangeLabel = (d: number) => (d === 0 ? t('全部') : d === 365 ? t('1 年') : t('{n} 天', { n: d }))

export function Metrics() {
  const { schema, changed, toast } = useApp()
  const [type, setType] = useState(hashParam('type') || '体重')
  const [days, setDays] = useState<number>(90)
  const from = days ? addDays(today(), -days + 1) : undefined
  const { data: rows } = useLoad(() => api.metricSeries(type, from), [type, days])
  const unit = tx(schema.metric_units[type]) || ''
  const isBP = type === '血压'

  const [v, setV] = useState('')
  const [v2, setV2] = useState('')
  const [ctx, setCtx] = useState('')
  const [at, setAt] = useState('')
  const [error, setError] = useState('')

  async function add(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      await api.create('metrics', { metric_type: type, value: v, value2: isBP ? v2 : undefined, context: ctx, measured_at: at || nowLocal() })
      setV(''); setV2(''); setCtx(''); setAt('')
      changed()
      toast(t('已记录'))
    } catch (err: any) {
      setError(err.message)
    }
  }

  const vals = (rows || []).map((r) => r.value as number)
  const last = rows?.[rows.length - 1]
  const first = rows?.[0]
  const delta = last && first && rows!.length > 1 ? last.value - first.value : null

  return (
    <>
      <div className="page-head">
        <div className="titles"><h1>{t('身体数据')}</h1><div className="sub">{t('体重、血压、血糖等日常测量')}</div></div>
      </div>
      <div className="pills" style={{ marginBottom: 18 }}>
        {schema.metric_types.map((m) => (
          <button key={m} className={'pill' + (m === type ? ' on' : '')} onClick={() => setType(m)}>{tx(m)}</button>
        ))}
      </div>
      <div className="stack">
        <section className="card">
          <div className="card-head">
            <h2>{tx(type)}</h2>
            <div className="seg">
              {RANGES.map((d) => <button key={d} className={days === d ? 'on' : ''} onClick={() => setDays(d)}>{rangeLabel(d)}</button>)}
            </div>
          </div>
          {last && (
            <div className="row" style={{ gap: 28, marginBottom: 14, alignItems: 'flex-end' }}>
              <div><div className="sub">{t('最近')}</div><div><span style={{ fontSize: 30, fontWeight: 700, letterSpacing: '-0.02em' }} className="num">{metricValue(last)}</span> <span className="muted">{unit}</span></div></div>
              {delta != null && !isBP && (
                <div><div className="sub">{t('区间变化')}</div><div className="num" style={{ fontSize: 17, fontWeight: 600 }}>{delta > 0 ? '+' : ''}{fmtNum(delta)} {unit}</div></div>
              )}
              {vals.length > 1 && !isBP && (
                <div><div className="sub">{t('区间范围')}</div><div className="num" style={{ fontSize: 17, fontWeight: 600 }}>{fmtNum(Math.min(...vals))} – {fmtNum(Math.max(...vals))}</div></div>
              )}
              <div className="sub" style={{ marginLeft: 'auto' }}>{t('{n} 次测量', { n: rows!.length })}</div>
            </div>
          )}
          <LineChart unit={unit} names={isBP ? [t('收缩压'), t('舒张压')] : undefined}
            points={(rows || []).map((r) => ({ date: r.measured_at, v: r.value, v2: r.value2 }))} />
        </section>

        <section className="card">
          <div className="card-head"><h2>{t('记一笔')}</h2></div>
          <form className="row" onSubmit={add} style={{ gap: 10, alignItems: 'flex-end' }}>
            <label className="field" style={{ width: 120 }}>
              <span className="lbl">{paren(isBP ? t('收缩压') : t('数值'), unit)}</span>
              <input className="input num" inputMode="decimal" value={v} onChange={(e) => setV(e.target.value)} required />
            </label>
            {isBP && (
              <label className="field" style={{ width: 120 }}>
                <span className="lbl">{t('舒张压')}</span>
                <input className="input num" inputMode="decimal" value={v2} onChange={(e) => setV2(e.target.value)} />
              </label>
            )}
            <label className="field" style={{ width: 160 }}>
              <span className="lbl">{t('情境')}</span>
              <input className="input" placeholder={t('如 空腹、晨起')} value={ctx} onChange={(e) => setCtx(e.target.value)} />
            </label>
            <label className="field" style={{ width: 220 }}>
              <span className="lbl">{t('时间（不填为现在）')}</span>
              <input className="input" type="datetime-local" value={at.replace(' ', 'T')} onChange={(e) => setAt(e.target.value.replace('T', ' '))} />
            </label>
            <button className="btn primary" style={{ height: 38 }}><Plus size={16} />{t('记录|button')}</button>
          </form>
          {error && <div className="error" style={{ marginTop: 10 }}>{error}</div>}
        </section>

        {rows && rows.length > 0 ? (
          <section className="card flush">
            <div className="list">
              {[...rows].reverse().slice(0, 100).map((r) => (
                <div key={r.id} className="row-item no-ico" onClick={() => openRecord('metrics', r.id)}>
                  <div className="body"><span className="val" style={{ fontSize: 15 }}>{metricValue(r)}</span> <span className="muted small">{tx(r.unit)}</span></div>
                  <span className="side">{r.context && <span className="tag">{r.context}</span>}<span className="nowrap num">{fmtDate(r.measured_at)}</span></span>
                </div>
              ))}
            </div>
          </section>
        ) : rows && <div className="card"><Empty compact>{t('这段时间没有{type}的记录', { type: tx(type) })}</Empty></div>}
      </div>
    </>
  )
}
