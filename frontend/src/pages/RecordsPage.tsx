import { useState, type ReactNode } from 'react'
import { Plus, Search } from 'lucide-react'
import { api, fmtDate, fmtNum, flagClass, metricValue, type Entity, type Rec } from '../api'
import { EntityForm } from '../components/EntityForm'
import { Modal } from '../components/Modal'
import { openRecord, recTitle, useApp, useLoad, useRefOptions } from '../state'
import { Empty, KIND, KindIcon, kindStyle } from '../ui'

const SUBTITLE: Record<string, string> = {
  conditions: '诊断过的、正在处理的、长期存在的健康问题',
  symptoms: '每一次不舒服：什么时候、多严重、可能的诱因',
  visits: '门诊、复诊、急诊、住院和线上问诊',
  medications: '正在吃的、吃过的药，剂量和效果',
  reports: '体检、化验、影像和病理报告',
}

// A list page for any record type: search, filter by status, add.
export function RecordsPage({ entityName, extra }: { entityName: string; extra?: ReactNode }) {
  const { entity, toast } = useApp()
  const e = entity(entityName)
  const [q, setQ] = useState('')
  const statusField = e.fields.find((f) => f.name === 'status' && f.type === 'enum')
  const [status, setStatus] = useState('')
  const [adding, setAdding] = useState(false)
  const { data, error, loading } = useLoad(() => api.list(entityName, { q, status, limit: 500 }), [entityName, q, status])

  return (
    <>
      <div className="page-head">
        <div className="titles">
          <h1>{e.label}</h1>
          <div className="sub">{SUBTITLE[entityName]}{data ? ` · 共 ${data.total} 条` : ''}</div>
        </div>
        {extra}
        <button className="btn primary" onClick={() => setAdding(true)}><Plus size={16} />添加{e.label}</button>
      </div>
      <div className="row" style={{ marginBottom: 16, gap: 12 }}>
        <div className="input-search">
          <Search size={15} />
          <input className="input" placeholder={`搜索${e.label}`} value={q} onChange={(ev) => setQ(ev.target.value)} />
        </div>
        {statusField && (
          <div className="seg">
            {['', ...statusField.options!].map((o) => (
              <button key={o} className={status === o ? 'on' : ''} onClick={() => setStatus(o)}>{o || '全部'}</button>
            ))}
          </div>
        )}
      </div>
      {error && <div className="error">{error}</div>}
      {!loading && data?.rows.length === 0 ? (
        <div className="card">
          <Empty icon={KIND[entityName]?.icon}>
            {q || status ? '没有符合条件的记录' : <>还没有{e.label}。点右上角添加，<br />或者在 Claude 里说一句就能记下。</>}
          </Empty>
        </div>
      ) : (
        <div className="card flush"><RecordList entity={e} rows={data?.rows || []} grouped /></div>
      )}
      {adding && (
        <Modal title={`添加${e.label}`} icon={<KindIcon kind={entityName} />} onClose={() => setAdding(false)}>
          <EntityForm entity={e} onCancel={() => setAdding(false)} onSaved={(r) => { setAdding(false); toast('已添加'); openRecord(e.name, r.id) }} />
        </Modal>
      )}
    </>
  )
}

function monthOf(s: string) {
  const [y, m] = s.split('-')
  return y && m ? `${y}年${Number(m)}月` : '未填日期'
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
        const meta = listFields.filter((f) => r[f.name] != null && r[f.name] !== '' && f.type !== 'enum' && f.type !== 'bool')
        return (
          <div key={r.id}>
            {head}
            <div className="row-item" onClick={() => openRecord(e.name, r.id)}>
              <KindIcon kind={e.name} />
              <div className="body">
                <div className="title">{e.name === 'metrics' ? `${r.metric_type} ${metricValue(r)} ${r.unit || ''}` : recTitle(e, r)}</div>
                <div className="meta">
                  {meta.map((f, i) => (
                    <span key={f.name}>{i > 0 && ' · '}{f.type === 'ref' ? <RefName entity={f.ref!} id={r[f.name]} /> : f.name === 'severity' ? `程度 ${r[f.name]}/10` : `${fmtNum(r[f.name])}${f.unit || ''}`}</span>
                  ))}
                </div>
              </div>
              <span className="side">
                {enums.map((f) => (
                  <span key={f.name} className={'tag ' + (f.name === 'flag' ? flagClass(r[f.name]) : 'k')} style={f.name === 'flag' ? undefined : kindStyle(e.name)}>{r[f.name]}</span>
                ))}
                {date && <span className="nowrap num" style={{ minWidth: 64, textAlign: 'right' }}>{fmtDate(date.slice(0, 10))}</span>}
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
