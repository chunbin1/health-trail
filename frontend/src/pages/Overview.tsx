import { useState, type ReactNode } from 'react'
import { Check, ChevronRight, Plus, SlidersHorizontal, Sparkles } from 'lucide-react'
import { addDays, api, fmtDate, fmtNum, metricValue, MONTHS, nowLocal, today, type MedDay, type Rec } from '../api'
import { EntityForm } from '../components/EntityForm'
import { tell } from '../components/Dialog'
import { Modal } from '../components/Modal'
import { NotifyWarning } from '../components/NotifyWarning'
import { LabValue, TakeDose } from '../components/RecordDrawer'
import { isEN, noun, t, tr, tx, type Key } from '../i18n'
import { openRecord, recTitle, useApp, useLoad } from '../state'
import { Empty, IconButton, KindIcon, RangeBar, Ring, Sparkline, Switch, kindStyle } from '../ui'
import { HabitGrid } from './Checkins'

const WEEK = ['日', '一', '二', '三', '四', '五', '六']
const WEEK_EN = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const QUICK: [string, Key][] = [
  ['symptoms', '症状|quick'], ['visits', '就诊|quick'], ['medications', '用药|quick'], ['appointments', '预约|quick'], ['metrics', '身体数据|quick'], ['conditions', '健康问题|quick'],
]

function greeting() {
  const h = new Date().getHours()
  return t(h < 5 ? '夜深了' : h < 11 ? '早上好' : h < 13 ? '中午好' : h < 18 ? '下午好' : '晚上好')
}

function todayLine(d: Date) {
  return isEN()
    ? `${WEEK_EN[d.getDay()]}, ${MONTHS[d.getMonth()]} ${d.getDate()}`
    : `${d.getMonth() + 1}月${d.getDate()}日 星期${WEEK[d.getDay()]}`
}

// Timeline titles are put together by the server in Chinese; the parts
// that are schema text or a dose's wording are translated here.
function timelineTitle(type: string, title: string) {
  if (!isEN()) return title
  if (type === 'metrics') return title.split(' ').map((w) => tx(w)).join(' ')
  const dose = type === 'doses' && /^服用 (.*) (\S+) 颗$/.exec(title)
  if (dose) return t('服用 {name} {amount}', { name: dose[1], amount: t('{n} 颗', { n: dose[2] }) })
  return tx(title)
}

export function Overview() {
  const { entity, toast } = useApp()
  const { data: o, error } = useLoad(() => api.overview(), [])
  const { data: timeline } = useLoad(() => api.timeline(10), [])
  const types = o?.latest_metrics.map((m) => m.metric_type).join(',') || ''
  const { data: series } = useLoad(async () => {
    const out: Record<string, number[]> = {}
    await Promise.all(types.split(',').filter(Boolean).map(async (t) => {
      const rows = await api.metricSeries(t)
      out[t] = rows.slice(-20).map((r) => r.value)
    }))
    return out
  }, [types])
  const [adding, setAdding] = useState<string | null>(null)
  // the body metrics to leave off this page, kept in settings.json
  const { data: info } = useLoad(() => api.info(), [])
  const [hidden, setHidden] = useState<string[] | null>(null)
  const hide = hidden ?? info?.hidden_metrics ?? []
  const [choosing, setChoosing] = useState(false)
  const d = new Date()

  if (error) return <div className="error">{error}</div>
  if (!o) return null
  const empty = Object.values(o.counts).every((n) => n === 0)
  const done = o.today_checkins.filter((h) => h.checkin).length

  return (
    <>
      <div className="hello">
        <div className="titles">
          <div className="sub">{todayLine(d)}</div>
          <h1>{greeting()}</h1>
        </div>
        <div className="row">
          {QUICK.map(([k, label]) => (
            <button key={k} className="btn" onClick={() => setAdding(k)}>
              <Plus size={14} style={{ color: `var(--c-${k})` }} strokeWidth={2.6} />{t(label)}
            </button>
          ))}
        </div>
      </div>

      <NotifyWarning />

      {empty && (
        <div className="notice" style={{ marginBottom: 20 }}>
          <Sparkles size={18} />
          <span>{tr('这里会汇总你的健康状况。先到 {link} 接好，然后把体检报告截图发给 AI，或者说「帮我记一下：今天头痛，大概 4 分」，它会直接写进来。', { link: <a href="#/connect">{t('连接 AI|link')}</a> })}</span>
        </div>
      )}

      <div className="stats">
        <Stat kind="conditions" href="#/conditions" n={o.active_conditions.length} label={t('进行中的健康问题')} />
        <Stat kind="medications" href="#/medications" n={o.current_medications.length} label={t('在用药物')} />
        <Stat kind="lab_results" href="#/reports?tab=trends" n={o.abnormal_labs_latest.length} label={t('异常指标')} warn={o.abnormal_labs_latest.length > 0} />
        <a className="stat" href="#/checkins" style={kindStyle('checkins')}>
          <div className="row" style={{ justifyContent: 'space-between' }}>
            <KindIcon kind="checkins" />
            <Ring value={done} total={o.today_checkins.length} size={30} />
          </div>
          <div><div className="n">{done}<small>/{o.today_checkins.length}</small></div><div className="k">{t('今日打卡')}</div></div>
        </a>
      </div>

      <div className="stack">
        <section className="card">
          <div className="card-head"><h2>{t('今日打卡')}</h2><a href="#/checkins" className="card-link">{t('管理')}<ChevronRight size={14} /></a></div>
          <HabitGrid days={o.today_checkins} date={o.today} />
        </section>

        <div className={o.meds_today.length ? 'grid-2' : 'stack'}>
          {o.meds_today.length > 0 && <MedsToday days={o.meds_today} />}
          <Panel title={t('接下来的预约')} rows={o.upcoming_appointments} kind="appointments" more="#/appointments"
            empty={t('没有待去的预约。跟 AI 说「下周三下午 3 点做胃镜」就能记下，到时间前会提醒')}
            render={(r) => <>
              <div className="body">
                <div className="title">{r.title}</div>
                <div className="meta">{[fmtDate(r.scheduled_at), r.place].filter(Boolean).join(' · ')}</div>
              </div>
              <When at={r.scheduled_at} />
            </>} />
        </div>

        <div className="grid-2">
          <section className="card">
            <div className="card-head">
              <h2>{t('需要留意的指标')}</h2>
              <a href="#/reports?tab=trends" className="card-link">{t('趋势')}<ChevronRight size={14} /></a>
            </div>
            {o.abnormal_labs_latest.length === 0
              ? <Empty compact>{t('各项指标最近一次的结果都在参考范围内')}</Empty>
              : (
                <table className="data">
                  <tbody>
                    {o.abnormal_labs_latest.slice(0, 7).map((r) => (
                      <tr key={r.id} className="clickable" onClick={() => openRecord('lab_results', r.id)}>
                        <td><div>{r.item_name}</div><div className="muted small">{fmtDate(r.test_date)}</div></td>
                        <td className="num" style={{ textAlign: 'right' }}><LabValue r={{ ...r, unit: null }} /></td>
                        <td style={{ width: '38%' }}><RangeBar value={r.value_num} low={r.ref_low} high={r.ref_high} flag={r.flag} /></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            {o.abnormal_labs_latest.length > 7 && <a href="#/reports?tab=trends" className="muted small" style={{ display: 'inline-block', marginTop: 8 }}>{t('还有 {n} 项', { n: o.abnormal_labs_latest.length - 7 })}</a>}
          </section>

          <div className="stack" style={{ gap: 16 }}>
            <Panel title={t('进行中的健康问题')} rows={o.active_conditions} kind="conditions" more="#/conditions" empty={t('没有进行中的健康问题')}
              render={(r) => <><div className="body title">{r.name}</div>{r.status && <span className="tag k" style={kindStyle('conditions')}>{tx(r.status)}</span>}</>} />
            <Panel title={t('在用药物')} rows={o.current_medications} kind="medications" more="#/medications" empty={t('没有在用的药物')}
              render={(r) => <>
                <div className="body">
                  <div className="title">{r.name}</div>
                  <div className="meta">{[r.dosage, r.frequency].filter(Boolean).join(' · ')}</div>
                  {r.last_taken && <div className="meta">{t('上次 {date} · {total}', { date: fmtDate(r.last_taken), total: t('共 {n} 颗', { n: fmtNum(r.taken_total) }) })}</div>}
                </div>
                <TakeDose med={r} />
              </>} />
            {o.upcoming_follow_ups.length > 0 && (
              <Panel title={t('待复诊')} rows={o.upcoming_follow_ups} kind="visits" empty=""
                render={(r) => <><div className="body"><div className="title">{fmtDate(r.follow_up_date)}</div><div className="meta">{recTitle(entity('visits'), r)}</div></div></>} />
            )}
          </div>
        </div>

        {o.latest_metrics.length > 0 && (
          <section className="card">
            <div className="card-head">
              <h2>{t('身体数据')}<IconButton small label={t('选择首页显示的指标')} onClick={() => setChoosing(true)}><SlidersHorizontal size={14} /></IconButton></h2>
              <a href="#/metrics" className="card-link">{t('全部')}<ChevronRight size={14} /></a>
            </div>
            {o.latest_metrics.every((m) => hide.includes(m.metric_type)) && (
              <div className="row muted small" style={{ gap: 8 }}>{t('首页没有选中的指标。')}<button className="btn ghost sm" onClick={() => setChoosing(true)}>{t('选择指标')}</button></div>
            )}
            <div className="tiles">
              {o.latest_metrics.filter((m) => !hide.includes(m.metric_type)).map((m) => (
                <div key={m.metric_type} className="tile" style={kindStyle('metrics')}
                  onClick={() => (location.hash = '/metrics?type=' + encodeURIComponent(m.metric_type))}>
                  <div className="k">{tx(m.metric_type)}</div>
                  <div className="v">{metricValue(m)}<small>{tx(m.unit)}</small></div>
                  <Sparkline values={series?.[m.metric_type] || []} />
                  <div className="d">{fmtDate(m.measured_at)}</div>
                </div>
              ))}
            </div>
          </section>
        )}

        <section className="card">
          <div className="card-head"><h2>{t('最近记录')}</h2></div>
          {timeline?.length === 0 && <Empty compact>{t('还没有记录')}</Empty>}
          <div className="list timeline">
            {timeline?.map((it) => (
              <div key={it.type + it.id} className="row-item" onClick={() => openRecord(it.type, it.id)}>
                <KindIcon kind={it.type} size="sm" />
                <div className="body title" style={{ fontWeight: 500 }}>{timelineTitle(it.type, it.title)}</div>
                <span className="side"><span className="tag">{tx(entity(it.type)?.label)}</span><span className="nowrap" style={{ minWidth: 70, textAlign: 'right' }}>{fmtDate(it.date.slice(0, 10))}</span></span>
              </div>
            ))}
          </div>
        </section>
      </div>

      {choosing && (
        <Modal title={t('选择首页显示的指标')} icon={<KindIcon kind="metrics" />} onClose={() => setChoosing(false)}>
          <div className="modal-body">
            {o.latest_metrics.map((m) => {
              const on = !hide.includes(m.metric_type)
              return (
                <div key={m.metric_type} className="row" style={{ gap: 8, flexWrap: 'nowrap', padding: '8px 0' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>{tx(m.metric_type)}</div>
                  <Switch checked={on} label={tx(m.metric_type)} onChange={(v) => {
                    const next = v ? hide.filter((x) => x !== m.metric_type) : [...hide, m.metric_type]
                    setHidden(next)
                    api.setHiddenMetrics(next).catch((e) => toast(e.message))
                  }} />
                </div>
              )
            })}
            <div className="small muted" style={{ marginTop: 8 }}>{t('关掉的指标只是不在首页显示，记录都还在「身体数据」里。')}</div>
          </div>
          <div className="modal-foot"><span className="spacer" /><button className="btn primary" onClick={() => setChoosing(false)}>{t('完成')}</button></div>
        </Modal>
      )}
      {adding && (
        <Modal title={t('添加{label}', { label: noun(entity(adding)) })} icon={<KindIcon kind={adding} />} onClose={() => setAdding(null)}>
          <EntityForm entity={entity(adding)} onCancel={() => setAdding(null)} onSaved={() => { setAdding(null); toast(t('已添加')) }} />
        </Modal>
      )}
    </>
  )
}

// How soon an appointment is: today, tomorrow, or already past (still to be
// recorded).
function When({ at }: { at: string }) {
  const day = at.slice(0, 10)
  if (at < nowLocal()) return <span className="tag">{t('已过')}</span>
  if (day === today()) return <span className="tag k" style={kindStyle('appointments')}>{t('今天')}</span>
  if (day === addDays(today(), 1)) return <span className="tag k" style={kindStyle('appointments')}>{t('明天')}</span>
  return null
}

// Today's medicines with reminder times: each time taken (with when) or
// not. A time not taken yet is a button that logs that dose: at this moment
// if it is the time nearest now, else at the time itself, so a dose made up
// late lands on its own time and not on the next one.
function MedsToday({ days }: { days: MedDay[] }) {
  const { changed, toast } = useApp()
  const [busy, setBusy] = useState('')
  const now = nowLocal()
  const minutes = (hm: string) => Number(hm.slice(0, 2)) * 60 + Number(hm.slice(3, 5))

  async function take(m: Rec, times: string[], time: string) {
    const nearest = times.reduce((a, b) => (Math.abs(minutes(b) - minutes(now.slice(11))) < Math.abs(minutes(a) - minutes(now.slice(11))) ? b : a))
    const at = nearest === time ? 'now' : `${today()} ${time}`
    setBusy(m.id + time)
    try {
      await api.create('doses', { medication_id: m.id, taken_at: at })
      changed()
      toast(t('已记录：{name} {time} 这一次', { name: m.name, time }))
    } catch (e: any) {
      tell(e.message)
    } finally {
      setBusy('')
    }
  }

  return (
    <section className="card" style={{ paddingBottom: 8 }}>
      <div className="card-head" style={{ marginBottom: 4 }}>
        <h2><KindIcon kind="medications" size="sm" />{t('今天的用药')}</h2>
        <a href="#/medications" className="card-link">{t('全部')}<ChevronRight size={14} /></a>
      </div>
      <div className="small muted">{t('吃了哪一顿，就点哪个时间')}</div>
      <div className="list">
        {days.map(({ medication: m, slots }) => (
          <div key={m.id} className="row-item no-ico" onClick={() => openRecord('medications', m.id)}>
            <div className="body">
              <div className="title">{[m.name, m.dosage].filter(Boolean).join(' ')}</div>
              <div className="slots">
                {slots.map((sl) => sl.dose ? (
                  <span key={sl.time} className="slot done" title={t('{time} 吃的', { time: String(sl.dose.taken_at).slice(11) })}>
                    <Check size={12} strokeWidth={3} />{sl.time}
                  </span>
                ) : (
                  <button key={sl.time} type="button" className={'slot' + (sl.time < now.slice(11) ? ' late' : '')}
                    disabled={busy === m.id + sl.time} title={t('记 {time} 这一次吃药', { time: sl.time })}
                    onClick={(e) => { e.stopPropagation(); take(m, slots.map((x) => x.time), sl.time) }}>
                    {sl.time}
                  </button>
                ))}
              </div>
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function Stat({ kind, href, n, label, warn }: { kind: string; href: string; n: number; label: string; warn?: boolean }) {
  return (
    <a className="stat" href={href} style={kindStyle(kind)}>
      <KindIcon kind={kind} />
      <div><div className={'n' + (warn ? ' flag-high' : '')}>{fmtNum(n)}</div><div className="k">{label}</div></div>
    </a>
  )
}

function Panel({ title, rows, kind, render, more, empty }: { title: string; rows: Rec[]; kind: string; render: (r: Rec) => ReactNode; more?: string; empty: string }) {
  return (
    <section className="card" style={{ paddingBottom: rows.length ? 8 : 18 }}>
      <div className="card-head" style={{ marginBottom: 4 }}>
        <h2><KindIcon kind={kind} size="sm" />{title}</h2>
        {more && <a href={more} className="card-link">{t('全部')}<ChevronRight size={14} /></a>}
      </div>
      {rows.length === 0 && <Empty compact>{empty}</Empty>}
      <div className="list">
        {rows.slice(0, 4).map((r) => (
          <div key={r.id} className="row-item no-ico" onClick={() => openRecord(kind, r.id)}>{render(r)}</div>
        ))}
      </div>
    </section>
  )
}
