import { useRef, useState, type MouseEvent } from 'react'
import { ArrowDown, ArrowUp, File, FileImage, FileText, Paperclip, Pencil, Pill, Plus, Trash2, Upload, X } from 'lucide-react'
import { api, fmtDate, fmtNum, flagClass, labValue, metricValue, today, type Entity, type Field, type Rec } from '../api'
import { noun, t, tx, type Key } from '../i18n'
import { closeRecord, openRecord, recTitle, useApp, useLoad, useRefOptions } from '../state'
import { Empty, IconButton, KindIcon, RangeBar } from '../ui'
import { EntityForm } from './EntityForm'
import { ask, tell } from './Dialog'
import { Drawer, Modal } from './Modal'

const CHILD_LABELS: Record<string, Key> = {
  symptoms: '症状记录', visits: '就诊记录', medications: '用药', reports: '检查报告', lab_results: '检验指标', checkins: '打卡记录', doses: '服药记录', appointments: '预约',
}

// The detail of one record: its fields, what hangs off it, its files.
export function RecordDrawer({ entityName, id }: { entityName: string; id: number }) {
  const { entity, changed, toast } = useApp()
  const e = entity(entityName)
  const { data, error } = useLoad(() => api.get(entityName, id), [entityName, id])
  const [editing, setEditing] = useState(false)
  const [adding, setAdding] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  if (!e) return null
  const r = data?.record

  async function remove() {
    if (!r || !(await ask(t('删除这条{label}？{extra}此操作不可撤销。', { label: noun(e), extra: entityName === 'reports' ? t('报告里的所有指标也会一起删除。') : '' }), { ok: t('删除'), danger: true }))) return
    try {
      await api.remove(entityName, id)
      changed()
      toast(t('已删除'))
      closeRecord()
    } catch (err: any) {
      tell(err.message)
    }
  }

  async function upload(files: FileList | null) {
    if (!files) return
    for (const f of Array.from(files)) {
      try {
        await api.upload(entityName, id, f)
      } catch (err: any) {
        tell(t('{name}：{error}', { name: f.name, error: err.message }))
      }
    }
    changed()
  }

  async function removeFile(aid: number) {
    if (!(await ask(t('删除这个附件？'), { ok: t('删除'), danger: true }))) return
    try {
      await api.removeAttachment(aid)
      changed()
    } catch (err: any) {
      tell(err.message)
    }
  }

  // the Mac window can't follow a link to a new window, so there the file
  // opens in its Mac app; a browser or the phone follows the link
  async function openFile(ev: MouseEvent, aid: number) {
    if (!document.documentElement.classList.contains('mac-window')) return
    ev.preventDefault()
    try {
      const { revealed } = await api.openAttachment(aid)
      if (revealed) toast(t('这种文件不能直接打开，已在访达中显示'))
    } catch (err: any) {
      tell(err.message)
    }
  }

  // a past check-in is history: it can be looked at, not changed
  const locked = entityName === 'checkins' && r?.date !== today()
  const head = r && !locked && (
    <>
      <button className="btn sm" onClick={() => setEditing(true)}><Pencil size={14} />{t('编辑')}</button>
      {/* a check-in item is archived, never deleted: its check-ins stay */}
      {entityName !== 'habits' && <IconButton label={t('删除')} className="danger" onClick={remove}><Trash2 size={16} /></IconButton>}
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
              <div className="sub">{tx(e.label)}{e.dateKey && r[e.dateKey] ? ' · ' + fmtDate(r[e.dateKey]) : ''}</div>
              <h1>{entityName === 'metrics' ? `${tx(r.metric_type)} ${metricValue(r)} ${tx(r.unit) || ''}` : recTitle(e, r)}</h1>
              {entityName === 'lab_results' && (
                <div className="row" style={{ marginTop: 6 }}><LabValue r={r} /><span className="muted small">{t('参考 {range}', { range: r.ref_range || '—' })}</span></div>
              )}
            </div>
          </div>

          <dl className="kv">
            {e.fields.filter((f) => f.name !== e.titleKey && f.name !== e.dateKey && r[f.name] != null && r[f.name] !== '').map((f) => (
              <FieldRow key={f.name} f={f} r={r} />
            ))}
          </dl>

          {entityName === 'appointments' && (!r.status || r.status === '待去') && <AppointmentDone appt={r} />}

          {entityName === 'reports' && (
            <section className="card">
              <div className="card-head">
                <h2>{t('检验指标')}</h2>
                <IconButton label={t('添加一项')} small onClick={() => setAdding('lab_results')}><Plus size={16} /></IconButton>
              </div>
              <LabTable rows={data.lab_results || []} />
            </section>
          )}

          {Object.entries(data)
            .filter(([k, v]) => Array.isArray(v) && k !== 'attachments' && k !== 'lab_results')
            .map(([k, rows]) => (
              <section key={k} className="card">
                <div className="card-head">
                  <h2>{CHILD_LABELS[k] ? t(CHILD_LABELS[k]) : k}<span className="muted small">{k === 'doses' ? doseTotal(rows as Rec[]) : (rows as Rec[]).length || ''}</span></h2>
                  {k === 'doses' && <TakeDose med={r} />}
                  {k !== 'checkins' && <IconButton label={t('添加{label}', { label: noun(entity(k)) })} small onClick={() => setAdding(k)}><Plus size={16} /></IconButton>}
                </div>
                <ChildList entity={entity(k)} rows={rows as Rec[]} />
              </section>
            ))}

          <section className="card">
            <div className="card-head">
              <h2>{t('附件')}</h2>
              <button className="btn sm" onClick={() => fileRef.current?.click()}><Upload size={14} />{t('上传')}</button>
              <input ref={fileRef} type="file" multiple hidden accept="image/*,application/pdf,.heic"
                onChange={(ev) => { upload(ev.target.files); ev.target.value = '' }} />
            </div>
            {data.attachments.length === 0 && <Empty compact>{t('报告原件、处方照片等可以放在这里')}</Empty>}
            {data.attachments.map((a) => {
              const Icon = a.mime?.startsWith('image/') ? FileImage : a.mime === 'application/pdf' ? FileText : File
              return (
                <div className="attach" key={a.id}>
                  <Icon size={18} className="muted" />
                  <a href={`/api/attachments/${a.id}`} target="_blank" rel="noreferrer" onClick={(ev) => openFile(ev, a.id)}>{a.filename}</a>
                  <span className="small muted nowrap">{Math.max(1, Math.round(a.size / 1024))} KB</span>
                  <IconButton label={t('删除附件')} small className="danger" onClick={() => removeFile(a.id)}><X size={15} /></IconButton>
                </div>
              )
            })}
          </section>
          <div className="small muted" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <Paperclip size={12} />{t('创建于 {created} · 更新于 {updated}', { created: r.created_at, updated: r.updated_at })}
          </div>
        </>
      )}
      {editing && r && (
        <Modal title={t('编辑{label}', { label: noun(e) })} icon={<KindIcon kind={entityName} />} onClose={() => setEditing(false)}>
          <EntityForm entity={e} record={r} onCancel={() => setEditing(false)} onSaved={() => { setEditing(false); toast(t('已保存')) }} />
        </Modal>
      )}
      {adding && r && (
        <Modal title={t('添加{label}', { label: noun(entity(adding)) })} icon={<KindIcon kind={adding} />} onClose={() => setAdding(null)}>
          <EntityForm entity={entity(adding)} defaults={childDefaults(entityName, adding, r)}
            onCancel={() => setAdding(null)} onSaved={() => { setAdding(null); toast(t('已添加')) }} />
        </Modal>
      )}
    </Drawer>
  )
}

const VISIT_TYPES: Record<string, string> = { 复诊: '复诊', 门诊: '门诊', 体检: '体检' }

// Once an appointment has happened, what came of it is recorded as a visit
// or a report, prefilled from it; the appointment is then marked done and
// linked to that record.
function AppointmentDone({ appt: a }: { appt: Rec }) {
  const { entity, changed, toast } = useApp()
  const [making, setMaking] = useState<'visits' | 'reports' | null>(null)
  const date = String(a.scheduled_at).slice(0, 10)
  const defaults = making === 'visits'
    ? { visit_date: date, hospital: a.place, visit_type: VISIT_TYPES[a.appt_type], chief_complaint: a.title, condition_id: a.condition_id }
    : { report_date: date, title: a.title, institution: a.place, report_type: a.appt_type === '体检' ? '体检' : undefined }

  async function saved(rec: Rec) {
    setMaking(null)
    await api.update('appointments', a.id, { status: '已完成', [making === 'visits' ? 'visit_id' : 'report_id']: rec.id })
    changed()
    toast(t('已记录，预约标记为已完成'))
    openRecord(making!, rec.id)
  }

  async function cancel() {
    await api.update('appointments', a.id, { status: '已取消' })
    changed()
    toast(t('预约已取消，不会再提醒'))
  }

  return (
    <section className="card">
      <div className="card-head"><h2>{t('去过之后')}</h2></div>
      <div className="small muted" style={{ marginBottom: 10 }}>{t('把结果记下来，这条预约会标记为已完成')}</div>
      <div className="row">
        <button className="btn sm" onClick={() => setMaking('visits')}><KindIcon kind="visits" size="sm" />{t('记为就诊')}</button>
        <button className="btn sm" onClick={() => setMaking('reports')}><KindIcon kind="reports" size="sm" />{t('记为检查报告')}</button>
        <span className="spacer" />
        <button className="btn sm ghost" onClick={cancel}>{t('取消预约')}</button>
      </div>
      {making && (
        <Modal title={t('添加{label}', { label: noun(entity(making)) })} icon={<KindIcon kind={making} />} onClose={() => setMaking(null)}>
          <EntityForm entity={entity(making)} defaults={defaults} onCancel={() => setMaking(null)} onSaved={saved} />
        </Modal>
      )}
    </section>
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
      toast(t('已记录：{name} {amount} · {time}', { name: med.name, amount: t('{n} 颗', { n: d.amount }), time: String(d.taken_at).slice(11) }))
    } catch (err: any) {
      tell(err.message)
    } finally {
      setBusy(false)
    }
  }
  return <button className="btn sm" disabled={busy} onClick={take}><Pill size={14} />{t('吃了')}</button>
}

function doseTotal(rows: Rec[]) {
  return rows.length ? t('共 {n} 颗', { n: fmtNum(rows.reduce((n, d) => n + (d.amount ?? 1), 0)) }) : ''
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
      <dt>{tx(f.label)}</dt>
      <dd>
        {f.type === 'ref' ? <RefLink field={f} id={v} />
          : f.type === 'date' || f.type === 'datetime' ? fmtDate(v)
          : f.type === 'bool' ? (v ? t('是') : t('否'))
          : f.name === 'flag' ? <span className={'tag ' + flagClass(v)}>{tx(v)}</span>
          : f.type === 'enum' ? <span className="tag">{tx(v)}</span>
          : `${fmtNum(v)}${f.unit ? ' ' + tx(f.unit) : ''}`}
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
  if (rows.length === 0) return <Empty compact>{t('暂无')}</Empty>
  return (
    <div className="list">
      {rows.slice(0, 50).map((c) => (
        <div className="row-item no-ico" key={c.id} onClick={() => openRecord(e.name, c.id)}>
          <div className="body title">{e.name === 'metrics' ? `${tx(c.metric_type)} ${metricValue(c)}` : e.name === 'doses' ? `${t('{n} 颗', { n: fmtNum(c.amount) })}${c.notes ? ' · ' + c.notes : ''}` : recTitle(e, c)}</div>
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
      {labValue(r)}{Arrow && <Arrow size={13} strokeWidth={2.6} aria-label={tx(r.flag)} />}
      {r.unit && <span className="muted small" style={{ fontWeight: 400, marginLeft: 4 }}>{r.unit}</span>}
    </span>
  )
}

export function LabTable({ rows, onPick }: { rows: Rec[]; onPick?: (name: string) => void }) {
  if (rows.length === 0) return <Empty compact>{t('还没有指标。把报告截图发给 AI 录入，或手动添加。')}</Empty>
  const groups = new Map<string, Rec[]>()
  for (const r of rows) {
    const k = r.panel || ''
    if (!groups.has(k)) groups.set(k, [])
    groups.get(k)!.push(r)
  }
  return (
    <div className="table-wrap">
      <table className="data">
        <thead><tr><th>{t('项目')}</th><th>{t('结果')}</th><th style={{ width: '28%' }}>{t('参考范围')}</th><th className="nowrap" /></tr></thead>
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
