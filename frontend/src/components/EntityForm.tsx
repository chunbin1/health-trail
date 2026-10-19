import { useState, type FormEvent } from 'react'
import { api, nowLocal, today, type Entity, type Field, type Rec } from '../api'
import { recTitle, useApp, useRefOptions } from '../state'
import { EmojiPicker, Switch } from '../ui'

// A form for any record type, drawn from the schema. `defaults` pre-fills
// a new record (e.g. the condition a symptom belongs to). Rendered inside a
// Modal: it supplies the dialog's body and footer.
export function EntityForm({ entity, record, defaults, onSaved, onCancel }: {
  entity: Entity
  record?: Rec
  defaults?: Record<string, any>
  onSaved: (r: Rec) => void
  onCancel: () => void
}) {
  const { changed } = useApp()
  const [values, setValues] = useState<Record<string, any>>(() => {
    if (record) return { ...record }
    const v: Record<string, any> = { ...defaults }
    for (const f of entity.fields) {
      if (v[f.name] !== undefined) continue
      if (f.name === entity.dateKey && f.type === 'date') v[f.name] = today()
      if (f.name === entity.dateKey && f.type === 'datetime') v[f.name] = nowLocal()
      if (f.type === 'enum' && f.name === 'status' && f.options) v[f.name] = f.options[0]
    }
    return v
  })
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const set = (k: string, v: any) => setValues((s) => ({ ...s, [k]: v }))
  // archiving is done from the record's page, not when creating it
  const fields = entity.fields.filter((f) => !(f.type === 'bool' && !record))

  async function submit(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    const body: Record<string, any> = {}
    for (const f of fields) {
      const v = values[f.name]
      if (record) {
        if (v !== record[f.name]) body[f.name] = v ?? ''
      } else if (v !== undefined && v !== '') body[f.name] = v
    }
    try {
      const saved = record ? await api.update(entity.name, record.id, body) : await api.create(entity.name, body)
      changed()
      onSaved(saved)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <div className="modal-body">
        <div className="form">
          {fields.map((f) => {
            if (f.type === 'bool')
              return (
                <div key={f.name} className="field inline full">
                  <span className="lbl">{f.label}</span>
                  <Switch checked={!!values[f.name]} onChange={(v) => set(f.name, v)} label={f.label} />
                </div>
              )
            const wide = f.type === 'longtext' || (entity.name === 'habits' && f.name === 'icon')
            return (
              <label key={f.name} className={'field' + (wide ? ' full' : '')}>
                <span className="lbl">
                  {f.label}{f.unit ? `（${f.unit}）` : ''}{f.required && <span className="req">*</span>}
                </span>
                <Input field={f} entity={entity} value={values[f.name]} onChange={(v) => set(f.name, v)} />
                {f.hint && !(entity.name === 'habits' && f.name === 'icon') && <span className="hint">{f.hint}</span>}
              </label>
            )
          })}
        </div>
        {error && <div className="error" style={{ marginTop: 14 }}>{error}</div>}
      </div>
      <div className="modal-foot">
        <span className="spacer" />
        <button type="button" className="btn" onClick={onCancel}>取消</button>
        <button className="btn primary" disabled={saving}>{saving ? '保存中…' : '保存'}</button>
      </div>
    </form>
  )
}

function Input({ field: f, entity, value, onChange }: { field: Field; entity: Entity; value: any; onChange: (v: any) => void }) {
  const v = value ?? ''
  if (entity.name === 'habits' && f.name === 'icon') return <EmojiPicker value={v} onChange={onChange} />
  switch (f.type) {
    case 'longtext':
      return <textarea className="input" value={v} placeholder={f.hint} onChange={(e) => onChange(e.target.value)} />
    case 'int':
    case 'real':
      return <input className="input num" inputMode="decimal" value={v} onChange={(e) => onChange(e.target.value)} />
    case 'date':
      return <input className="input" type="date" value={v} onChange={(e) => onChange(e.target.value)} />
    case 'datetime':
      return (
        <input className="input" type="datetime-local" value={toLocalInput(v)}
          onChange={(e) => onChange(e.target.value.replace('T', ' '))} />
      )
    case 'time':
      return <input className="input" type="time" value={v} onChange={(e) => onChange(e.target.value)} style={{ maxWidth: 160 }} />
    case 'enum':
      if (f.options!.length <= 4)
        return (
          <div className="seg" role="radiogroup">
            {f.options!.map((o) => (
              <button type="button" key={o} className={v === o ? 'on' : ''} onClick={() => onChange(v === o && !f.required ? '' : o)}>{o}</button>
            ))}
          </div>
        )
      return (
        <select className="input" value={v} onChange={(e) => onChange(e.target.value)}>
          {!f.required && <option value="">未选择</option>}
          {f.options!.map((o) => <option key={o}>{o}</option>)}
        </select>
      )
    case 'ref':
      return <RefSelect field={f} value={v} onChange={onChange} />
    default:
      return <input className="input" value={v} onChange={(e) => onChange(e.target.value)} />
  }
}

function toLocalInput(v: string) {
  if (!v) return ''
  return v.length === 10 ? v + 'T00:00' : v.replace(' ', 'T')
}

function RefSelect({ field, value, onChange }: { field: Field; value: any; onChange: (v: any) => void }) {
  const { entity } = useApp()
  const rows = useRefOptions(field.ref)
  const e = entity(field.ref!)
  return (
    <select className="input" value={value ?? ''} onChange={(ev) => onChange(ev.target.value === '' ? '' : Number(ev.target.value))}>
      <option value="">不关联</option>
      {rows.map((r) => (
        <option key={r.id} value={r.id}>
          {recTitle(e, r)}{e.dateKey && r[e.dateKey] ? `（${String(r[e.dateKey]).slice(0, 10)}）` : ''}
        </option>
      ))}
    </select>
  )
}
