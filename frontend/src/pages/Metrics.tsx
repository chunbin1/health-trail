import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Pencil, Plus, SlidersHorizontal } from 'lucide-react'
import { addDays, api, fmtDate, fmtNum, metricValue, nowLocal, today } from '../api'
import { DateTimeInput } from '../components/DateTimeInput'
import { EntityForm } from '../components/EntityForm'
import { LineChart } from '../components/LineChart'
import { ArrangeList } from '../components/ArrangeList'
import { Modal } from '../components/Modal'
import { noun, paren, t, tx } from '../i18n'
import { hashParam, openRecord, useApp, useListChoice, useLoad, useMetricKinds } from '../state'
import { Empty, IconButton, KindIcon } from '../ui'

const RANGES = [30, 90, 365, 0]
// shown from the start, before the person has chosen, with every metric
// that has readings
const KEY_METRICS = ['体重', 'BMI', '血压', '血糖']
const rangeLabel = (d: number) => (d === 0 ? t('全部') : d === 365 ? t('1 年') : t('{n} 天', { n: d }))

export function Metrics() {
  const { entity, changed, toast } = useApp()
  const mk = useMetricKinds()
  const [type, setType] = useState(hashParam('type') || '体重')
  const [days, setDays] = useState<number>(90)
  const from = days ? addDays(today(), -days + 1) : undefined
  const { data: rows } = useLoad(() => api.metricSeries(type, from), [type, days])
  const unit = tx(mk.units[type]) || ''
  const isBP = type === '血压'
  // BMI is worked out from the weights and the height: nothing to log here
  const derived = mk.derived.includes(type)
  const { data: heights } = useLoad(() => (derived ? api.metricSeries('身高') : Promise.resolve([])), [derived])
  const height = heights?.[heights.length - 1]
  const kind = mk.kinds.find((k) => k.name === type)
  const kindId = useRef<number>()
  if (kind) kindId.current = kind.id
  const [addingKind, setAddingKind] = useState(false)
  // the pills in the person's order, as on the 今天 page; before a choice,
  // the key ones and those with readings
  const { data: o } = useLoad(() => api.overview(), [])
  const withData = new Set(o?.latest_metrics.map((m) => m.metric_type))
  const list = useListChoice('metrics_page', mk.types, (m) => KEY_METRICS.includes(m) || withData.has(m))
  // one opened by a link shows while it's chosen
  const shown = list.ordered.filter((m) => list.shown(m) || m === type)
  const [choosing, setChoosing] = useState(false)
  // the metric was renamed (follow it) or deleted (show the first one) from
  // its drawer. Only when the list reloads: a metric just added is chosen
  // before it's in it.
  useEffect(() => {
    if (!mk.loaded || mk.types.includes(type)) return
    const renamed = mk.kinds.find((k) => k.id === kindId.current)
    setType(renamed ? renamed.name : mk.types[0])
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mk])

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
        <div className="titles"><h1>{t('身体数据')}<IconButton small label={t('选择直接显示的指标')} onClick={() => setChoosing(true)}><SlidersHorizontal size={16} /></IconButton></h1><div className="sub">{t('体重、血压、血糖等日常测量')}</div></div>
      </div>
      {/* drawn once it's known which are shown, so nothing jumps */}
      <div className="pills" style={{ marginBottom: 18, minHeight: 34 }}>
        {list.loaded && o && <>
          {shown.map((m) => (
            <button key={m} className={'pill' + (m === type ? ' on' : '')} onClick={() => setType(m)}>{tx(m)}</button>
          ))}
          <button className="pill" onClick={() => setAddingKind(true)}><Plus size={13} />{t('新指标')}</button>
        </>}
      </div>
      <div className="stack">
        <section className="card">
          <div className="card-head">
            <h2>{tx(type)}{kind && (
              <IconButton small label={t('修改这个指标')} onClick={() => openRecord('metric_kinds', kind.id)}><Pencil size={14} /></IconButton>
            )}</h2>
            <div className="seg">
              {RANGES.map((d) => <button key={d} className={days === d ? 'on' : ''} onClick={() => setDays(d)}>{rangeLabel(d)}</button>)}
            </div>
          </div>
          {last && (
            <div className="row" style={{ gap: 28, marginBottom: 14, alignItems: 'flex-end' }}>
              <div><div className="sub">{t('最近')}</div><div><span style={{ fontSize: 30, fontWeight: 700, letterSpacing: '-0.02em' }} className="num">{metricValue(last)}</span> <span className="muted">{unit}</span>{last.category && <span className="tag" style={{ marginLeft: 8 }}>{tx(last.category)}</span>}</div></div>
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

        {derived ? (
          <section className="card">
            <div className="card-head"><h2>{t('自动计算')}</h2></div>
            <div className="small muted" style={{ lineHeight: 1.7 }}>
              <div>{t('BMI = 体重（kg）÷ 身高（m）的平方。每次记体重，都按那时的身高算一次，不用单独记。')}</div>
              <div>{t('中国成人标准：低于 18.5 偏瘦，18.5–23.9 正常，24–27.9 超重，28 及以上肥胖。')}</div>
            </div>
            <div className="row" style={{ gap: 8, marginTop: 12 }}>
              <button className="btn sm" onClick={() => setType('身高')}>
                {height ? t('身高 {n}', { n: fmtNum(height.value) + ' ' + tx(height.unit || 'cm') }) : t('先记一次身高')}
              </button>
              <button className="btn sm" onClick={() => setType('体重')}>{t('记体重')}</button>
            </div>
          </section>
        ) : (
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
              <DateTimeInput value={at} onChange={setAt} />
            </label>
            <button className="btn primary" style={{ height: 38 }}><Plus size={16} />{t('记录|button')}</button>
          </form>
          {error && <div className="error" style={{ marginTop: 10 }}>{error}</div>}
        </section>
        )}

        {rows && rows.length > 0 ? (
          <section className="card flush">
            <div className="list">
              {/* a lab result of the same name is shown here, kept in its report */}
              {[...rows].reverse().slice(0, 100).map((r) => (
                <div key={r.source + r.id + (r.from || '')} className="row-item no-ico" onClick={() => openRecord(r.source, r.id)}>
                  <div className="body"><span className="val" style={{ fontSize: 15 }}>{metricValue(r)}</span> <span className="muted small">{tx(r.unit)}</span></div>
                  <span className="side">
                    {r.category && <span className="tag">{tx(r.category)}</span>}
                    {r.source === 'lab_results' && <span className="tag" title={r.report_title || undefined}>{t('化验单')}</span>}
                    {r.context && <span className="tag">{r.context}</span>}
                    <span className="nowrap num">{fmtDate(r.measured_at)}</span>
                  </span>
                </div>
              ))}
            </div>
          </section>
        ) : rows && <div className="card"><Empty compact>{derived && heights && !height ? t('记一次身高，就能按体重算出 BMI') : t('这段时间没有{type}的记录', { type: tx(type) })}</Empty></div>}
      </div>

      {choosing && (
        <ArrangeList title={t('选择直接显示的指标')} icon={<KindIcon kind="metrics" />}
          items={list.ordered.map((m) => ({ key: m, label: tx(m) }))} shown={list.shown} onChange={list.save}
          note={t('拖动左边的把手调整顺序。关掉的指标只是不在这一页列出，记录都还在，随时可以在这里打开。')} onClose={() => setChoosing(false)} />
      )}
      {addingKind && (
        <Modal title={t('添加{label}', { label: noun(entity('metric_kinds')) })} icon={<KindIcon kind="metric_kinds" />} onClose={() => setAddingKind(false)}>
          <EntityForm entity={entity('metric_kinds')} onCancel={() => setAddingKind(false)}
            onSaved={(r) => { setAddingKind(false); setType(r.name); toast(t('已添加')) }} />
        </Modal>
      )}
    </>
  )
}
