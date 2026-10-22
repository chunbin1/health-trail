import { useState, type ReactNode } from 'react'
import { Plus, Search } from 'lucide-react'
import { api, fmtDate, fmtMonth, fmtNum, flagClass, metricValue, today, type Entity, type Rec } from '../api'
import { EntityForm } from '../components/EntityForm'
import { Modal } from '../components/Modal'
import { isEN, noun, t, tr, tx, type Key } from '../i18n'
import { openRecord, recTitle, useApp, useLoad, useRefOptions } from '../state'
import { Empty, KIND, KindIcon, kindStyle } from '../ui'

const SUBTITLE: Record<string, Key> = {
  conditions: '诊断过的、正在处理的、长期存在的健康问题',
  symptoms: '每一次不舒服：什么时候、多严重、可能的诱因',
  visits: '门诊、复诊、急诊、住院和线上问诊',
  medications: '正在吃的、吃过的药，剂量和效果',
  reports: '体检、化验、影像和病理报告',
  appointments: '约好的检查、复诊和体检，到时间前会提醒',
}

// A list page for any record type: search, filter by status, add. `end`
// goes after the add button, at the head's right edge (a tab switcher that
// must not move when the buttons beside it change).
export function RecordsPage({ entityName, end }: { entityName: string; end?: ReactNode }) {
  const { entity, toast } = useApp()
  const e = entity(entityName)
  const [q, setQ] = useState('')
  const statusField = e.fields.find((f) => f.name === 'status' && f.type === 'enum')
  const [status, setStatus] = useState('')
  const [adding, setAdding] = useState(false)
  const { data, error, loading } = useLoad(() => api.list(entityName, { q, status, limit: 500 }), [entityName, q, status])

  return (
    <>
      <div className="page-top drag">
      <div className="page-head">
        <div className="titles">
          <h1>{tx(e.label)}</h1>
          <div className="sub">{SUBTITLE[entityName] && t(SUBTITLE[entityName])}{data ? t(' · 共 {n} 条', { n: data.total }) : ''}</div>
        </div>
        <button className="btn primary" onClick={() => setAdding(true)}><Plus size={16} />{t('添加{label}', { label: noun(e) })}</button>
        {end}
      </div>
      <div className="row page-tools" style={{ gap: 12 }}>
        <div className="input-search">
          <Search size={15} />
          <input className="input" placeholder={t('搜索{label}', { label: plural(e) })} value={q} onChange={(ev) => setQ(ev.target.value)} />
        </div>
        {statusField && (
          <div className="seg">
            {['', ...statusField.options!].map((o) => (
              <button key={o} className={status === o ? 'on' : ''} onClick={() => setStatus(o)}>{o ? tx(o) : t('全部')}</button>
            ))}
          </div>
        )}
      </div>
      </div>
      {error && <div className="error">{error}</div>}
      {!loading && data?.rows.length === 0 ? (
        <div className="card">
          <Empty icon={KIND[entityName]?.icon}>
            {q || status ? t('没有符合条件的记录') : tr('还没有{label}。点右上角添加，{br}或者跟 AI 说一句就能记下。', { label: plural(e), br: <br /> })}
          </Empty>
        </div>
      ) : (
        entityName === 'appointments' ? <AppointmentLists entity={e} rows={data?.rows || []} />
          : <div className="card flush"><RecordList entity={e} rows={data?.rows || []} grouped /></div>
      )}
      {adding && (
        <Modal title={t('添加{label}', { label: noun(e) })} icon={<KindIcon kind={entityName} />} onClose={() => setAdding(false)}>
          <EntityForm entity={e} onCancel={() => setAdding(false)} onSaved={(r) => { setAdding(false); toast(t('已添加')); openRecord(e.name, r.id) }} />
        </Modal>
      )}
    </>
  )
}

// Appointments still to go to come first, soonest at the top; the rest
// (past, done, cancelled) after, latest first.
function AppointmentLists({ entity, rows }: { entity: Entity; rows: Rec[] }) {
  const day = today()
  const isAhead = (r: Rec) => String(r.scheduled_at) >= day && (!r.status || r.status === '待去')
  const ahead = rows.filter(isAhead).reverse()
  const rest = rows.filter((r) => !isAhead(r))
  return (
    <div className="stack">
      {ahead.length > 0 && <div className="card flush"><div className="group-label">{t('接下来')}</div><RecordList entity={entity} rows={ahead} /></div>}
      {rest.length > 0 && <div className="card flush"><div className="group-label">{t('以往')}</div><RecordList entity={entity} rows={rest} /></div>}
    </div>
  )
}

// plural names the entity's records mid-sentence: 「搜索用药」, "Search medications".
const plural = (e: Entity) => (isEN() ? tx(e.label).toLowerCase() : e.label)

function monthOf(s: string) {
  const [y, m] = s.split('-')
  return y && m ? fmtMonth(y, m) : t('未填日期')
}

// Rows with the kind's icon, a title, a line of detail, and the status and
// date on the right. `grouped` puts a month heading between months.
export function RecordList({ entity: e, rows, grouped }: { entity: Entity; rows: Rec[]; grouped?: boolean }) {
  const listFields = e.fields.filter((f) => f.list && f.name !== e.titleKey && f.name !== e.dateKey)
  let lastMonth = ''
  return (
    <div className="list">
      {rows.map((r) => {
        const date = e.dateKey ? String(r[e.dateKey] ?? '') : ''
        const month = monthOf(date)
        const head = grouped && e.dateKey && month !== lastMonth ? <div className="group-label">{month}</div> : null
        lastMonth = month
        const enums = listFields.filter((f) => f.type === 'enum' && r[f.name] && f.name !== 'metric_type')
        // an appointment's date and time go under its title, leaving the title room beside its tags
        const when = e.name === 'appointments' && date ? fmtDate(date) : ''
        const meta = listFields.filter((f) => r[f.name] != null && r[f.name] !== '' && f.type !== 'enum' && f.type !== 'bool')
        return (
          <div key={r.id}>
            {head}
            <div className="row-item" onClick={() => openRecord(e.name, r.id)}>
              <KindIcon kind={e.name} />
              <div className="body">
                <div className="title">{e.name === 'metrics' ? `${tx(r.metric_type)} ${metricValue(r)} ${tx(r.unit) || ''}` : recTitle(e, r)}</div>
                <div className="meta">
                  {when}
                  {meta.map((f, i) => (
                    <span key={f.name}>{(i > 0 || when) && ' · '}{f.type === 'ref' ? <RefName entity={f.ref!} id={r[f.name]} /> : f.name === 'severity' ? t('程度 {n}/10', { n: r[f.name] }) : `${fmtNum(r[f.name])}${tx(f.unit) || ''}`}</span>
                  ))}
                </div>
              </div>
              <span className="side">
                {enums.map((f) => (
                  <span key={f.name} className={'tag ' + (f.name === 'flag' ? flagClass(r[f.name]) : 'k')} style={f.name === 'flag' ? undefined : kindStyle(e.name)}>{tx(r[f.name])}</span>
                ))}
                {date && !when && <span className="nowrap num" style={{ minWidth: 64, textAlign: 'right' }}>{fmtDate(date.slice(0, 10))}</span>}
              </span>
            </div>
          </div>
        )
      })}
    </div>
  )
}

function RefName({ entity: name, id }: { entity: string; id: number }) {
  const { entity } = useApp()
  const rows = useRefOptions(name)
  const t = rows.find((x) => x.id === id)
  return t ? <>{recTitle(entity(name), t)}</> : null
}
