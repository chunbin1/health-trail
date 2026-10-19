import { useState, type ReactNode } from 'react'
import { ChevronRight, Plus, Sparkles } from 'lucide-react'
import { api, fmtDate, fmtNum, metricValue, type Rec } from '../api'
import { EntityForm } from '../components/EntityForm'
import { Modal } from '../components/Modal'
import { LabValue, TakeDose } from '../components/RecordDrawer'
import { openRecord, recTitle, useApp, useLoad } from '../state'
import { Empty, KindIcon, RangeBar, Ring, Sparkline, kindStyle } from '../ui'
import { HabitGrid } from './Checkins'

const WEEK = ['日', '一', '二', '三', '四', '五', '六']
const QUICK = [
  ['symptoms', '症状'], ['visits', '就诊'], ['medications', '用药'], ['metrics', '身体数据'], ['conditions', '健康问题'],
] as const

function greeting() {
  const h = new Date().getHours()
  return h < 5 ? '夜深了' : h < 11 ? '早上好' : h < 13 ? '中午好' : h < 18 ? '下午好' : '晚上好'
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
  const d = new Date()

  if (error) return <div className="error">{error}</div>
  if (!o) return null
  const empty = Object.values(o.counts).every((n) => n === 0)
  const done = o.today_checkins.filter((h) => h.checkin).length

  return (
    <>
      <div className="hello">
        <div className="titles">
          <div className="sub">{d.getMonth() + 1}月{d.getDate()}日 星期{WEEK[d.getDay()]}</div>
          <h1>{greeting()}</h1>
        </div>
        <div className="row">
          {QUICK.map(([k, label]) => (
            <button key={k} className="btn" onClick={() => setAdding(k)}>
              <Plus size={14} style={{ color: `var(--c-${k})` }} strokeWidth={2.6} />{label}
            </button>
          ))}
        </div>
      </div>

      {empty && (
        <div className="notice" style={{ marginBottom: 20 }}>
          <Sparkles size={18} />
          <span>这里会汇总你的健康状况。先到 <a href="#/connect">连接 Claude</a> 接好，然后把体检报告截图发给 Claude，或者说「帮我记一下：今天头痛，大概 4 分」，它会直接写进来。</span>
        </div>
      )}

      <div className="stats">
        <Stat kind="conditions" href="#/conditions" n={o.active_conditions.length} label="进行中的健康问题" />
        <Stat kind="medications" href="#/medications" n={o.current_medications.length} label="在用药物" />
        <Stat kind="lab_results" href="#/reports" n={o.abnormal_labs_latest.length} label="异常指标" warn={o.abnormal_labs_latest.length > 0} />
        <a className="stat" href="#/checkins" style={kindStyle('checkins')}>
          <div className="row" style={{ justifyContent: 'space-between' }}>
            <KindIcon kind="checkins" />
            <Ring value={done} total={o.today_checkins.length} size={30} />
          </div>
          <div><div className="n">{done}<small>/{o.today_checkins.length}</small></div><div className="k">今日打卡</div></div>
        </a>
      </div>

      <div className="stack">
        <section className="card">
          <div className="card-head"><h2>今日打卡</h2><a href="#/checkins" className="card-link">管理<ChevronRight size={14} /></a></div>
          <HabitGrid days={o.today_checkins} date={o.today} />
        </section>

        <div className="grid-2">
          <section className="card">
            <div className="card-head">
              <h2>需要留意的指标</h2>
              <a href="#/reports" className="card-link">趋势<ChevronRight size={14} /></a>
            </div>
            {o.abnormal_labs_latest.length === 0
              ? <Empty compact>各项指标最近一次的结果都在参考范围内</Empty>
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
            {o.abnormal_labs_latest.length > 7 && <div className="muted small" style={{ marginTop: 8 }}>还有 {o.abnormal_labs_latest.length - 7} 项</div>}
          </section>

          <div className="stack" style={{ gap: 16 }}>
            <Panel title="进行中的健康问题" rows={o.active_conditions} kind="conditions" more="#/conditions" empty="没有进行中的健康问题"
              render={(r) => <><div className="body title">{r.name}</div>{r.status && <span className="tag k" style={kindStyle('conditions')}>{r.status}</span>}</>} />
            <Panel title="在用药物" rows={o.current_medications} kind="medications" more="#/medications" empty="没有在用的药物"
              render={(r) => <>
                <div className="body">
                  <div className="title">{r.name}</div>
                  <div className="meta">{[r.dosage, r.frequency].filter(Boolean).join(' · ')}</div>
                  {r.last_taken && <div className="meta">上次 {fmtDate(r.last_taken)} · 共 {fmtNum(r.taken_total)} 颗</div>}
                </div>
                <TakeDose med={r} />
              </>} />
            {o.upcoming_follow_ups.length > 0 && (
              <Panel title="待复诊" rows={o.upcoming_follow_ups} kind="visits" empty=""
                render={(r) => <><div className="body"><div className="title">{fmtDate(r.follow_up_date)}</div><div className="meta">{recTitle(entity('visits'), r)}</div></div></>} />
            )}
          </div>
        </div>

        {o.latest_metrics.length > 0 && (
          <section className="card">
            <div className="card-head"><h2>身体数据</h2><a href="#/metrics" className="card-link">全部<ChevronRight size={14} /></a></div>
            <div className="tiles">
              {o.latest_metrics.map((m) => (
                <div key={m.id} className="tile" style={kindStyle('metrics')}
                  onClick={() => (location.hash = '/metrics?type=' + encodeURIComponent(m.metric_type))}>
                  <div className="k">{m.metric_type}</div>
                  <div className="v">{metricValue(m)}<small>{m.unit}</small></div>
                  <Sparkline values={series?.[m.metric_type] || []} />
                  <div className="d">{fmtDate(m.measured_at)}</div>
                </div>
              ))}
            </div>
          </section>
        )}

        <section className="card">
          <div className="card-head"><h2>最近记录</h2></div>
          {timeline?.length === 0 && <Empty compact>还没有记录</Empty>}
          <div className="list timeline">
            {timeline?.map((t) => (
              <div key={t.type + t.id} className="row-item" onClick={() => openRecord(t.type, t.id)}>
                <KindIcon kind={t.type} size="sm" />
                <div className="body title" style={{ fontWeight: 500 }}>{t.title}</div>
                <span className="side"><span className="tag">{entity(t.type)?.label}</span><span className="nowrap" style={{ width: 70, textAlign: 'right' }}>{fmtDate(t.date.slice(0, 10))}</span></span>
              </div>
            ))}
          </div>
        </section>
      </div>

      {adding && (
        <Modal title={`添加${entity(adding).label}`} icon={<KindIcon kind={adding} />} onClose={() => setAdding(null)}>
          <EntityForm entity={entity(adding)} onCancel={() => setAdding(null)} onSaved={() => { setAdding(null); toast('已添加') }} />
        </Modal>
      )}
    </>
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
        {more && <a href={more} className="card-link">全部<ChevronRight size={14} /></a>}
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
