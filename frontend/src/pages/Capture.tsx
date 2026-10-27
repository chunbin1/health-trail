import { useEffect, useState } from 'react'
import { Mic, X } from 'lucide-react'
import { api, type AssistantConfig, type DraftItem, type Field, type LabDraft } from '../api'
import { DateTimeInput } from '../components/DateTimeInput'
import { AssistantSetup } from '../components/AssistantSetup'
import { t, tx } from '../i18n'
import { openRecord, useApp, useLoad } from '../state'
import { IconButton, KindIcon } from '../ui'

const FLAGS = ['正常', '偏高', '偏低', '异常']

export function Capture() {
  const { entity, changed, toast } = useApp()
  const { data, error, reload } = useLoad(() => api.assistant(), [])
  const [cfg, setCfg] = useState<AssistantConfig | null>(null)
  const [showSetup, setShowSetup] = useState(false)
  const [text, setText] = useState('')
  const [items, setItems] = useState<DraftItem[] | null>(null)
  const [unclear, setUnclear] = useState('')
  const [listening, setListening] = useState(false)
  const [micBusy, setMicBusy] = useState(false)
  const [organizing, setOrganizing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [locked, setLocked] = useState(false)
  const [saved, setSaved] = useState<Record<number, { type?: string; id?: number }>>({})
  const [err, setErr] = useState('')
  const [open, setOpen] = useState<Record<number, boolean>>({})
  const view = cfg || data

  useEffect(() => {
    if (!listening) return
    const id = setInterval(() => {
      api.speech().then((s) => {
        if (s.text) setText(s.text)
        if (s.error) setErr(s.error)
        if (!s.listening) setListening(false)
      }, () => {})
    }, 400)
    return () => clearInterval(id)
  }, [listening])

  if (error) return <div className="error">{error}</div>
  if (!view) return null

  function resetPreview() {
    setItems(null)
    setUnclear('')
    setLocked(false)
    setSaved({})
    setOpen({})
    setErr('')
  }

  async function mic() {
    if (micBusy) return
    setMicBusy(true)
    setErr('')
    try {
      if (!listening) {
        const s = await api.speechStart()
        setListening(s.listening)
        if (s.error) setErr(s.error)
        if (s.text) setText(s.text)
      } else {
        const s = await api.speechStop()
        setListening(false)
        if (s.text) setText(s.text)
      }
    } catch (e: any) {
      setListening(false)
      setErr(e.message)
    } finally {
      setMicBusy(false)
    }
  }

  async function organize() {
    if (organizing || micBusy || locked) return
    setOrganizing(true)
    setErr('')
    try {
      let said = text
      if (listening) {
        const s = await api.speechStop()
        setListening(false)
        if (s.text) said = s.text
        setText(said)
      }
      if (!said.trim()) {
        setErr(t('先写一点内容，或点麦克风说。'))
        return
      }
      const d = await api.assistantDraft(said.trim())
      setItems(d.items || [])
      setUnclear(d.unclear || '')
      setSaved({})
      setOpen({})
      setLocked(false)
    } catch (e: any) {
      setErr(e.message)
    } finally {
      setOrganizing(false)
    }
  }

  async function save() {
    if (saving || locked || !items?.length) return
    if (items.some((it) => it.problem)) {
      setErr(t('请先改掉标出的问题，再保存。'))
      return
    }
    setSaving(true)
    setErr('')
    try {
      const res = await api.assistantCommit(items.map(cleanItem))
      const done: Record<number, { type?: string; id?: number }> = {}
      for (const s of res.saved || []) done[s.index] = { type: s.type, id: s.id }
      setSaved(done)
      if (!res.ok) {
        setErr(res.error || '')
        if ((res.saved || []).length) {
          setLocked(true)
          changed()
        }
        return
      }
      changed()
      toast(t('已记入档案'))
      setText('')
      resetPreview()
    } catch (e: any) {
      setErr(e.message)
    } finally {
      setSaving(false)
    }
  }

  function patch(i: number, next: DraftItem) {
    setItems((cur) => cur?.map((it, n) => (n === i ? { ...next, problem: '' } : it)) || null)
  }

  const micLabel = listening ? t('结束听写') : micBusy ? t('正在听…') : t('开始听')

  return (
    <>
      <div className="page-head">
        <div className="titles">
          <h1>{t('记一笔|nav')}</h1>
          <div className="sub">{t('说一句或打一段字，整理成记录，你确认以后才写入。')}</div>
        </div>
      </div>
      <div className="stack" style={{ maxWidth: 760 }}>
        <div className="notice">
          <Mic size={18} />
          <span>{t('声音只在这台 Mac 上识别，不会上传。整理时，这段文字，以及已有健康问题、用药和打卡项的名称，会发给你填写的接口。整理出来的内容不能代替医生的诊断。')}</span>
        </div>

        {(!view.configured || showSetup) && (
          <section className="card">
            {!view.configured && <p className="sub" style={{ marginTop: 0 }}>{t('还没有填写接口。填好以后就可以在这里说话或打字记档案。')}</p>}
            <AssistantSetup cfg={view} onSaved={(c) => { setCfg(c); setShowSetup(false); reload() }} />
          </section>
        )}

        {view.configured && !showSetup && (
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button type="button" className="btn ghost sm" onClick={() => setShowSetup(true)}>{t('修改接口')}</button>
          </div>
        )}

        {view.configured && (
          <section className="card">
            <textarea className="input" style={{ width: '100%' }} rows={4} value={text}
              placeholder={t('说或输入要记的事，例如：今天早上头痛，大概 4 分')}
              onChange={(e) => setText(e.target.value)} />
            <div className="row" style={{ marginTop: 12 }}>
              {view.speech && (
                <button type="button" className={'btn sm' + (listening ? ' primary' : '')} disabled={micBusy && !listening} onClick={mic}>
                  <Mic size={14} />{micLabel}
                </button>
              )}
              <button type="button" className="btn primary sm" disabled={organizing || locked || micBusy} onClick={organize}>
                {organizing ? t('正在整理…') : t('整理')}
              </button>
            </div>
          </section>
        )}

        {err && <div className="error">{err}</div>}
        {locked && <div className="notice warn"><span>{t('已经写进档案的已标出。为避免记重，这次不能再保存。还没写上的，请到对应的页面补记。')}</span></div>}
        {unclear && <div className="notice"><span>{unclear}</span></div>}

        {items && items.length === 0 && <div className="sub">{t('没有整理出可以保存的记录。')}</div>}

        {items && items.length > 0 && (
          <div className="stack">
            {items.map((it, i) => (
              <section key={i} className="card">
                <div className="card-head">
                  <KindIcon kind={iconKind(it)} size="sm" />
                  <h2 style={{ flex: 1, minWidth: 0 }}>{itemTitle(it, entity)}</h2>
                  {saved[i] && <span className="tag">{t('已保存')}</span>}
                  {!locked && (
                    <IconButton label={t('去掉')} small onClick={() => setItems(items.filter((_, n) => n !== i))}>
                      <X size={14} />
                    </IconButton>
                  )}
                </div>
                {it.problem && <div className="error" style={{ marginBottom: 8 }}>{it.problem}</div>}
                {saved[i]?.id && saved[i]?.type && (
                  <button type="button" className="btn ghost sm" onClick={() => openRecord(saved[i].type!, saved[i].id!)}>{t('已保存')}</button>
                )}
                {!locked && (
                  <button type="button" className="btn ghost sm" onClick={() => setOpen({ ...open, [i]: !open[i] })}>
                    {open[i] ? t('收起') : t('修改')}
                  </button>
                )}
                {(open[i] || it.problem) && !locked && (
                  <ItemEditor item={it} onChange={(next) => patch(i, next)} />
                )}
              </section>
            ))}
            <div className="row">
              <button type="button" className="btn primary" disabled={saving || locked} onClick={save}>
                {saving ? t('正在保存…') : t('保存到档案')}
              </button>
              <button type="button" className="btn" disabled={saving} onClick={resetPreview}>{t('放弃')}</button>
            </div>
          </div>
        )}
      </div>
    </>
  )
}

function iconKind(it: DraftItem) {
  if (it.kind === 'checkin') return 'checkins'
  if (it.kind === 'lab_report') return 'lab_results'
  return it.type || 'search'
}

function itemTitle(it: DraftItem, entity: (name: string) => { label: string; titleKey: string } | undefined) {
  if (it.kind === 'checkin') return it.habit || t('打卡')
  if (it.kind === 'lab_report') return String(it.report?.title || t('化验单'))
  const e = it.type ? entity(it.type) : undefined
  const title = e && it.fields ? it.fields[e.titleKey] : ''
  return String(title || it.summary || (e ? tx(e.label) : it.type || ''))
}

function cleanItem(it: DraftItem): DraftItem {
  const fields: Record<string, any> = {}
  for (const [k, v] of Object.entries(it.fields || {})) {
    if (v === '' || v == null) continue
    fields[k] = v
  }
  const { problem: _p, ...rest } = it
  return { ...rest, fields }
}

function ItemEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  if (item.kind === 'checkin') return <CheckinEditor item={item} onChange={onChange} />
  if (item.kind === 'lab_report') return <LabEditor item={item} onChange={onChange} />
  return <RecordEditor item={item} onChange={onChange} />
}

function RecordEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  const { entity } = useApp()
  const e = item.type ? entity(item.type) : undefined
  const [all, setAll] = useState(false)
  if (!e) return null
  const fields = item.fields || {}
  const shown = e.fields.filter((f) => all || f.required || (fields[f.name] != null && fields[f.name] !== ''))
  const set = (k: string, v: any) => onChange({ ...item, fields: { ...fields, [k]: v } })
  const hasCond = e.fields.some((f) => f.name === 'condition_id')
  const hasMed = e.fields.some((f) => f.name === 'medication_id')
  return (
    <div className="form" style={{ marginTop: 12 }}>
      {shown.map((f) => (
        <FieldInput key={f.name} field={f} value={fields[f.name]} onChange={(v) => set(f.name, v)} wide={f.type === 'longtext'} />
      ))}
      {hasCond && (
        <label className="field full">
          <span className="lbl">{t('相关健康问题的名称')}</span>
          <input className="input" value={item.condition_name || ''} onChange={(e) => onChange({ ...item, condition_name: e.target.value })} />
        </label>
      )}
      {hasMed && (
        <label className="field full">
          <span className="lbl">{t('对应药物的名称')}</span>
          <input className="input" value={item.medication_name || ''} onChange={(e) => onChange({ ...item, medication_name: e.target.value })} />
        </label>
      )}
      <div className="full">
        <button type="button" className="btn ghost sm" onClick={() => setAll(!all)}>{all ? t('收起') : t('全部字段')}</button>
      </div>
    </div>
  )
}

function FieldInput({ field, value, onChange, wide }: { field: Field; value: any; onChange: (v: any) => void; wide?: boolean }) {
  const v = value ?? ''
  if (field.type === 'bool') return null
  const box = field.type === 'date' || field.type === 'datetime' || field.type === 'time' ? (
    <DateTimeInput mode={field.type} value={String(v)} onChange={onChange} />
  ) : field.type === 'enum' ? (
    <select className="input" value={String(v)} onChange={(e) => onChange(e.target.value)}>
      {!field.required && <option value="">{t('未选择')}</option>}
      {(field.options || []).map((o) => <option key={o} value={o}>{tx(o)}</option>)}
    </select>
  ) : field.type === 'longtext' ? (
    <textarea className="input" value={String(v)} onChange={(e) => onChange(e.target.value)} />
  ) : field.type === 'int' || field.type === 'real' || field.type === 'ref' ? (
    <input className="input" inputMode="decimal" value={v === '' ? '' : String(v)}
      onChange={(e) => onChange(e.target.value === '' ? '' : Number(e.target.value))} />
  ) : (
    <input className="input" value={String(v)} onChange={(e) => onChange(e.target.value)} />
  )
  return (
    <label className={'field' + (wide ? ' full' : '')}>
      <span className="lbl">{tx(field.label)}{field.required && <span className="req">*</span>}</span>
      {box}
    </label>
  )
}

function CheckinEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  return (
    <div className="form" style={{ marginTop: 12 }}>
      <label className="field">
        <span className="lbl">{t('打卡项')}</span>
        <input className="input" value={item.habit || ''} onChange={(e) => onChange({ ...item, habit: e.target.value })} />
      </label>
      <label className="field">
        <span className="lbl">{t('数量')}</span>
        <input className="input" inputMode="decimal" value={item.value ?? ''} onChange={(e) => onChange({ ...item, value: e.target.value === '' ? null : Number(e.target.value) })} />
      </label>
      <label className="field full">
        <span className="lbl">{t('备注')}</span>
        <input className="input" value={item.notes || ''} onChange={(e) => onChange({ ...item, notes: e.target.value })} />
      </label>
    </div>
  )
}

function LabEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  const report = item.report || {}
  const setReport = (k: string, v: any) => onChange({ ...item, report: { ...report, [k]: v } })
  const results = item.results || []
  const setRow = (i: number, row: LabDraft) => onChange({ ...item, results: results.map((r, n) => (n === i ? row : r)) })
  return (
    <div className="form" style={{ marginTop: 12 }}>
      <label className="field">
        <span className="lbl">{t('标题')}</span>
        <input className="input" value={report.title || ''} onChange={(e) => setReport('title', e.target.value)} />
      </label>
      <label className="field">
        <span className="lbl">{tx('报告日期')}</span>
        <DateTimeInput mode="date" value={String(report.report_date || '')} onChange={(v) => setReport('report_date', v)} />
      </label>
      <label className="field">
        <span className="lbl">{tx('类型')}</span>
        <select className="input" value={report.report_type || ''} onChange={(e) => setReport('report_type', e.target.value)}>
          <option value="">{t('未选择')}</option>
          {['体检', '化验', '影像', '病理', '其他'].map((o) => <option key={o} value={o}>{tx(o)}</option>)}
        </select>
      </label>
      {results.map((r, i) => (
        <div key={i} className="full form" style={{ padding: 10, border: '1px solid var(--line)', borderRadius: 10 }}>
          <label className="field">
            <span className="lbl">{t('项目')}</span>
            <input className="input" value={r.item_name || ''} onChange={(e) => setRow(i, { ...r, item_name: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{t('结果')}</span>
            <input className="input" value={r.value ?? ''} onChange={(e) => setRow(i, { ...r, value: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{t('单位')}</span>
            <input className="input" value={r.unit || ''} onChange={(e) => setRow(i, { ...r, unit: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{t('参考范围')}</span>
            <input className="input" value={r.ref_range || ''} onChange={(e) => setRow(i, { ...r, ref_range: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{tx('标记')}</span>
            <select className="input" value={r.flag || ''} onChange={(e) => setRow(i, { ...r, flag: e.target.value })}>
              <option value="">{t('未选择')}</option>
              {FLAGS.map((f) => <option key={f} value={f}>{tx(f)}</option>)}
            </select>
          </label>
          <div className="field">
            <span className="lbl">&nbsp;</span>
            <button type="button" className="btn sm" onClick={() => onChange({ ...item, results: results.filter((_, n) => n !== i) })}>{t('删掉这条指标')}</button>
          </div>
        </div>
      ))}
      <div className="full">
        <button type="button" className="btn sm" onClick={() => onChange({ ...item, results: [...results, {}] })}>{t('添加一条指标')}</button>
      </div>
    </div>
  )
}
