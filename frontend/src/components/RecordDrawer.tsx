import { useRef, useState, type MouseEvent } from 'react'
import { ArrowDown, ArrowUp, File, FileImage, FileText, Paperclip, Pencil, Pill, Plus, Trash2, Upload, X } from 'lucide-react'
import { api, fmtDate, fmtNum, flagClass, labValue, metricValue, type Entity, type Field, type Rec } from '../api'
import { closeRecord, openRecord, recTitle, useApp, useLoad, useRefOptions } from '../state'
import { Empty, IconButton, KindIcon, RangeBar } from '../ui'
import { EntityForm } from './EntityForm'
import { LabImport } from './LabImport'
import { Drawer, Modal } from './Modal'

const CHILD_LABELS: Record<string, string> = {
  symptoms: '症状记录', visits: '就诊记录', medications: '用药', reports: '检查报告', lab_results: '检验指标', checkins: '打卡记录', doses: '服药记录',
}

// The detail of one record: its fields, what hangs off it, its files.
export function RecordDrawer({ entityName, id }: { entityName: string; id: number }) {
  const { entity, changed, toast } = useApp()
  const e = entity(entityName)
  const { data, error } = useLoad(() => api.get(entityName, id), [entityName, id])
  const [editing, setEditing] = useState(false)
  const [adding, setAdding] = useState<string | null>(null)
  const [importing, setImporting] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  if (!e) return null
  const r = data?.record

  async function remove() {
    if (!r || !confirm(`删除这条${e.label}？${entityName === 'reports' ? '报告里的所有指标也会一起删除。' : ''}此操作不可撤销。`)) return
    try {
      await api.remove(entityName, id)
      changed()
      toast('已删除')
      closeRecord()
    } catch (err: any) {
      alert(err.message)
    }
  }

  async function upload(files: FileList | null) {
    if (!files) return
    for (const f of Array.from(files)) {
      try {
        await api.upload(entityName, id, f)
      } catch (err: any) {
        alert(`${f.name}：${err.message}`)
      }
    }
    changed()
  }

  async function removeFile(aid: number) {
    if (!confirm('删除这个附件？')) return
    await api.removeAttachment(aid)
    changed()
  }

  const head = r && (
    <>
      <button className="btn sm" onClick={() => setEditing(true)}><Pencil size={14} />编辑</button>
      <IconButton label="删除" className="danger" onClick={remove}><Trash2 size={16} /></IconButton>
    </>
  )

  return (
    <Drawer onClose={closeRecord} head={head}>
      {error && <div className="error">{error}</div>}
      {r && (
        <>
          <div className="drawer-title">
            <KindIcon kind={entityName} size="lg" />
            <div style={{ minWidth: 0 }}>
              <div className="sub">{e.label}{e.dateKey && r[e.dateKey] ? ' · ' + fmtDate(r[e.dateKey]) : ''}</div>
              <h1>{entityName === 'metrics' ? `${r.metric_type} ${metricValue(r)} ${r.unit || ''}` : recTitle(e, r)}</h1>
              {entityName === 'lab_results' && (
                <div className="row" style={{ marginTop: 6 }}><LabValue r={r} /><span className="muted small">参考 {r.ref_range || '—'}</span></div>
              )}
            </div>
          </div>

          <dl className="kv">
            {e.fields.filter((f) => f.name !== e.titleKey && f.name !== e.dateKey && r[f.name] != null && r[f.name] !== '').map((f) => (
              <FieldRow key={f.name} f={f} r={r} />
            ))}
          </dl>

          {entityName === 'reports' && (
            <section className="card">
              <div className="card-head">
                <h2>检验指标</h2>
                <button className="btn sm" onClick={() => setImporting(true)}>粘贴导入</button>
                <IconButton label="添加一项" small onClick={() => setAdding('lab_results')}><Plus size={16} /></IconButton>
              </div>
              <LabTable rows={data.lab_results || []} />
            </section>
          )}

          {Object.entries(data)
            .filter(([k, v]) => Array.isArray(v) && k !== 'attachments' && k !== 'lab_results')
            .map(([k, rows]) => (
              <section key={k} className="card">
                <div className="card-head">
                  <h2>{CHILD_LABELS[k] || k}<span className="muted small">{k === 'doses' ? doseTotal(rows as Rec[]) : (rows as Rec[]).length || ''}</span></h2>
                  {k === 'doses' && <TakeDose med={r} />}
                  {k !== 'checkins' && <IconButton label={`添加${CHILD_LABELS[k]}`} small onClick={() => setAdding(k)}><Plus size={16} /></IconButton>}
                </div>
                <ChildList entity={entity(k)} rows={rows as Rec[]} />
              </section>
            ))}

          <section className="card">
            <div className="card-head">
              <h2>附件</h2>
              <button className="btn sm" onClick={() => fileRef.current?.click()}><Upload size={14} />上传</button>
              <input ref={fileRef} type="file" multiple hidden accept="image/*,application/pdf,.heic"
                onChange={(ev) => { upload(ev.target.files); ev.target.value = '' }} />
            </div>
            {data.attachments.length === 0 && <Empty compact>报告原件、处方照片等可以放在这里</Empty>}
            {data.attachments.map((a) => {
              const Icon = a.mime?.startsWith('image/') ? FileImage : a.mime === 'application/pdf' ? FileText : File
              return (
                <div className="attach" key={a.id}>
                  <Icon size={18} className="muted" />
                  <a href={`/api/attachments/${a.id}`} target="_blank" rel="noreferrer">{a.filename}</a>
                  <span className="small muted nowrap">{Math.max(1, Math.round(a.size / 1024))} KB</span>
                  <IconButton label="删除附件" small className="danger" onClick={() => removeFile(a.id)}><X size={15} /></IconButton>
                </div>
              )
            })}
          </section>
          <div className="small muted" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <Paperclip size={12} />创建于 {r.created_at} · 更新于 {r.updated_at}
          </div>
        </>
      )}
      {editing && r && (
        <Modal title={`编辑${e.label}`} icon={<KindIcon kind={entityName} />} onClose={() => setEditing(false)}>
          <EntityForm entity={e} record={r} onCancel={() => setEditing(false)} onSaved={() => { setEditing(false); toast('已保存') }} />
        </Modal>
      )}
      {adding && r && (
        <Modal title={`添加${entity(adding).label}`} icon={<KindIcon kind={adding} />} onClose={() => setAdding(null)}>
          <EntityForm entity={entity(adding)} defaults={childDefaults(entityName, adding, r)}
            onCancel={() => setAdding(null)} onSaved={() => { setAdding(null); toast('已添加') }} />
        </Modal>
      )}
      {importing && r && <LabImport report={r} onClose={() => setImporting(false)} onSaved={() => setImporting(false)} />}
    </Drawer>
  )
}

// One tap logs one pill of med taken now; another time or amount goes
// through the + form.
export function TakeDose({ med }: { med: Rec }) {
  const { changed, toast } = useApp()
  const [busy, setBusy] = useState(false)
  async function take(ev: MouseEvent) {
    ev.stopPropagation()
    setBusy(true)
    try {
      const d = await api.create('doses', { medication_id: med.id, taken_at: 'now' })
      changed()
      toast(`已记录：${med.name} ${fmtNum(d.amount)} 颗 · ${String(d.taken_at).slice(11)}`)
    } catch (err: any) {
      alert(err.message)
    } finally {
      setBusy(false)
    }
  }
  return <button className="btn sm" disabled={busy} onClick={take}><Pill size={14} />吃了</button>
}

function doseTotal(rows: Rec[]) {
  return rows.length ? `共 ${fmtNum(rows.reduce((n, d) => n + (d.amount ?? 1), 0))} 颗` : ''
}

function childDefaults(parent: string, child: string, r: Rec) {
  const key = { conditions: 'condition_id', visits: 'visit_id', reports: 'report_id', habits: 'habit_id', medications: 'medication_id' }[parent]
  const d: Record<string, any> = key ? { [key]: r.id } : {}
  if (child === 'lab_results') d.test_date = r.report_date
  return d
}

function FieldRow({ f, r }: { f: Field; r: Rec }) {
  const v = r[f.name]
  return (
    <div className="kv-row">
      <dt>{f.label}</dt>
      <dd>
        {f.type === 'ref' ? <RefLink field={f} id={v} />
          : f.type === 'date' || f.type === 'datetime' ? fmtDate(v)
          : f.type === 'bool' ? (v ? '是' : '否')
          : f.name === 'flag' ? <span className={'tag ' + flagClass(v)}>{v}</span>
          : f.type === 'enum' ? <span className="tag">{v}</span>
          : `${fmtNum(v)}${f.unit ? ' ' + f.unit : ''}`}
      </dd>
    </div>
  )
}

export function RefLink({ field, id }: { field: Field; id: number }) {
  const { entity } = useApp()
  const rows = useRefOptions(field.ref)
  const target = rows.find((x) => x.id === id)
  const e = entity(field.ref!)
  return (
    <a href="#" onClick={(ev) => { ev.preventDefault(); openRecord(field.ref!, id) }}>
      {target ? recTitle(e, target) : `#${id}`}
    </a>
  )
}

function ChildList({ entity: e, rows }: { entity: Entity; rows: Rec[] }) {
  if (rows.length === 0) return <Empty compact>暂无</Empty>
  return (
    <div className="list">
      {rows.slice(0, 50).map((c) => (
        <div className="row-item no-ico" key={c.id} onClick={() => openRecord(e.name, c.id)}>
          <div className="body title">{e.name === 'metrics' ? `${c.metric_type} ${metricValue(c)}` : e.name === 'doses' ? `${fmtNum(c.amount)} 颗${c.notes ? ' · ' + c.notes : ''}` : recTitle(e, c)}</div>
          {e.dateKey && <span className="side">{fmtDate(e.name === 'doses' ? c.taken_at : String(c[e.dateKey] ?? '').slice(0, 10))}</span>}
        </div>
      ))}
    </div>
  )
}

export function LabValue({ r }: { r: Rec }) {
  const Arrow = r.flag === '偏高' ? ArrowUp : r.flag === '偏低' ? ArrowDown : null
  return (
    <span className={'val ' + flagClass(r.flag)}>
      {labValue(r)}{Arrow && <Arrow size={13} strokeWidth={2.6} aria-label={r.flag} />}
      {r.unit && <span className="muted small" style={{ fontWeight: 400, marginLeft: 4 }}>{r.unit}</span>}
    </span>
  )
}

export function LabTable({ rows, onPick }: { rows: Rec[]; onPick?: (name: string) => void }) {
  if (rows.length === 0) return <Empty compact>还没有指标。可以粘贴导入，或把报告截图发给 Claude 录入。</Empty>
  const groups = new Map<string, Rec[]>()
  for (const r of rows) {
    const k = r.panel || ''
    if (!groups.has(k)) groups.set(k, [])
    groups.get(k)!.push(r)
  }
  return (
    <div className="table-wrap">
      <table className="data">
        <thead><tr><th>项目</th><th>结果</th><th style={{ width: '28%' }}>参考范围</th><th className="nowrap" /></tr></thead>
        <tbody>
          {[...groups.entries()].map(([panel, rs]) => [
            panel && groups.size > 1 && <tr key={'p' + panel} className="panel-row"><td colSpan={4}>{panel}</td></tr>,
            ...rs.map((r) => (
              <tr key={r.id} className="clickable" onClick={() => (onPick ? onPick(r.item_name) : openRecord('lab_results', r.id))}>
                <td>{r.item_name}</td>
                <td className="num"><LabValue r={r} /></td>
                <td><RangeBar value={r.value_num} low={r.ref_low} high={r.ref_high} flag={r.flag} /></td>
                <td className="muted num small nowrap">{r.ref_range}</td>
              </tr>
            )),
          ])}
        </tbody>
      </table>
    </div>
  )
}
