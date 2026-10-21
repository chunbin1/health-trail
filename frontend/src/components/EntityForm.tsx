import { Fragment, useState, type FormEvent } from 'react'
import { api, nowLocal, today, type Entity, type Field, type Rec } from '../api'
import { paren, t, tx } from '../i18n'
import { recTitle, useApp, useMetricKinds, useRefOptions } from '../state'
import { EmojiPicker, Switch } from '../ui'
import { DateTimeInput } from './DateTimeInput'

// The choice in a metric select that adds a new metric (metric_kinds) as the
// record is saved.
const NEW = '__new__'

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
  const mk = useMetricKinds()
  const [kind, setKind] = useState({ name: '', unit: '' })
  const [values, setValues] = useState<Record<string, any>>(() => {
    if (record) return { ...record }
    const v: Record<string, any> = { ...defaults }
    for (const f of entity.fields) {
      if (v[f.name] !== undefined) continue
      if (f.name === entity.dateKey && f.type === 'date') v[f.name] = today()
      if (f.name === entity.dateKey && f.type === 'datetime') v[f.name] = nowLocal()
      if (f.type === 'enum' && f.name === 'status' && f.options) v[f.name] = f.options[0]
      // a required select shows its first option, so start with that value
      if (f.type === 'enum' && f.required && f.options) v[f.name] = f.options[0]
    }
    return v
  })
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const set = (k: string, v: any) => setValues((s) => ({ ...s, [k]: v }))
  // body data: the second value is only for blood pressure (systolic/diastolic),
  // and the unit follows the metric, so it isn't typed in
  const metric = entity.name === 'metrics'
  const isNew = values.metric_type === NEW
  const isBP = metric && values.metric_type === '血压'
  const metricUnit = !metric ? '' : isNew ? kind.unit.trim() : mk.units[values.metric_type] || ''
  const label = (f: Field) =>
    isBP && f.name === 'value' ? t('收缩压') : isBP && f.name === 'value2' ? t('舒张压') : tx(f.label)
  const unitOf = (f: Field) => (metric && f.name.startsWith('value') ? tx(metricUnit) : tx(f.unit))
  const hidden = (f: Field) =>
    (f.type === 'bool' && !record) || // archiving is done from the record's page, not when creating it
    (metric && f.name === 'value2' && !isBP) ||
    (metric && f.name === 'unit')
  const fields = entity.fields.filter((f) => !hidden(f))

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
    // switched away from blood pressure: clear the hidden diastolic value
    if (metric && !isBP && record?.value2 != null) body.value2 = ''
    let added: Rec | undefined
    try {
      if (isNew) {
        added = await api.create('metric_kinds', { name: kind.name.trim(), unit: kind.unit.trim() })
        body.metric_type = added.name
      }
      const saved = record ? await api.update(entity.name, record.id, body) : await api.create(entity.name, body)
      changed()
      onSaved(saved)
    } catch (err: any) {
      // the new metric goes with the record it was made for
      if (added) await api.remove('metric_kinds', added.id).catch(() => {})
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
                  <span className="lbl">{tx(f.label)}</span>
                  <Switch checked={!!values[f.name]} onChange={(v) => set(f.name, v)} label={tx(f.label)} />
                </div>
              )
            const wide = f.type === 'longtext' || (entity.name === 'habits' && f.name === 'icon')
            const v = values[f.name]
            return (
              <Fragment key={f.name}>
                <label className={'field' + (wide ? ' full' : '')}>
                  <span className="lbl">
                    {paren(label(f), unitOf(f))}{f.required && <span className="req">*</span>}
                  </span>
                  {f.custom ? (
                    <select className="input" value={v ?? ''} onChange={(e) => set(f.name, e.target.value)}>
                      {!f.required && <option value="">{t('未选择')}</option>}
                      {[...f.options!, ...mk.kinds.map((k) => k.name as string)].concat(
                        // a value no longer among the choices still shows as itself
                        v && v !== NEW && mk.loaded && !f.options!.includes(v) && !mk.kinds.some((k) => k.name === v) ? [v] : [],
                      ).map((o) => <option key={o} value={o}>{tx(o)}</option>)}
                      <option value={NEW}>{t('＋ 新指标…')}</option>
                    </select>
                  ) : (
                    <Input field={f} entity={entity} value={v} onChange={(v) => set(f.name, v)} />
                  )}
                  {f.hint && !(entity.name === 'habits' && f.name === 'icon') && !(metric && f.name.startsWith('value')) &&
                    <span className="hint">{tx(f.hint)}</span>}
                </label>
                {f.custom && v === NEW && (
                  <div className="field full">
                    <span className="lbl">{t('新指标')}<span className="req">*</span></span>
                    <div className="new-kind">
                      <input className="input" autoFocus required aria-label={t('新指标的名称')} placeholder={t('名称，如：尿酸')}
                        value={kind.name} onChange={(e) => setKind({ ...kind, name: e.target.value })} />
                      <input className="input" aria-label={t('单位')} placeholder={t('单位')}
                        value={kind.unit} onChange={(e) => setKind({ ...kind, unit: e.target.value })} />
                    </div>
                    <span className="hint">{t('以后记这个指标会自动带上单位，也可以在身体数据页修改')}</span>
                  </div>
                )}
              </Fragment>
            )
          })}
        </div>
        {error && <div className="error" style={{ marginTop: 14 }}>{error}</div>}
      </div>
      <div className="modal-foot">
        <span className="spacer" />
        <button type="button" className="btn" onClick={onCancel}>{t('取消')}</button>
        <button className="btn primary" disabled={saving}>{saving ? t('保存中…') : t('保存')}</button>
      </div>
    </form>
  )
}

function Input({ field: f, entity, value, onChange }: { field: Field; entity: Entity; value: any; onChange: (v: any) => void }) {
  const v = value ?? ''
  if (entity.name === 'habits' && f.name === 'icon') return <EmojiPicker value={v} onChange={onChange} />
  switch (f.type) {
    case 'longtext':
      return <textarea className="input" value={v} placeholder={tx(f.hint)} onChange={(e) => onChange(e.target.value)} />
    case 'int':
    case 'real':
      return <input className="input num" inputMode="decimal" value={v} onChange={(e) => onChange(e.target.value)} />
    case 'date':
      return <DateTimeInput mode="date" value={v} onChange={onChange} required={f.required} />
    case 'datetime':
      return (
        <DateTimeInput value={v} onChange={onChange} required={f.required} />
      )
    case 'time':
      return <DateTimeInput mode="time" value={v} onChange={onChange} required={f.required} />
    case 'enum':
      if (f.options!.length <= 4)
        return (
          <div className="seg" role="radiogroup">
            {f.options!.map((o) => (
              <button type="button" key={o} className={v === o ? 'on' : ''} onClick={() => onChange(v === o && !f.required ? '' : o)}>{tx(o)}</button>
            ))}
          </div>
        )
      return (
        <select className="input" value={v} onChange={(e) => onChange(e.target.value)}>
          {!f.required && <option value="">{t('未选择')}</option>}
          {f.options!.map((o) => <option key={o} value={o}>{tx(o)}</option>)}
        </select>
      )
    case 'ref':
      return <RefSelect field={f} value={v} onChange={onChange} />
    default:
      return <input className="input" value={v} onChange={(e) => onChange(e.target.value)} />
  }
}

function RefSelect({ field, value, onChange }: { field: Field; value: any; onChange: (v: any) => void }) {
  const { entity } = useApp()
  const rows = useRefOptions(field.ref)
  const e = entity(field.ref!)
  return (
    <select className="input" value={value ?? ''} onChange={(ev) => onChange(ev.target.value === '' ? '' : Number(ev.target.value))}>
      <option value="">{t('不关联')}</option>
      {rows.map((r) => (
        <option key={r.id} value={r.id}>
          {paren(recTitle(e, r), e.dateKey && r[e.dateKey] ? String(r[e.dateKey]).slice(0, 10) : '')}
        </option>
      ))}
    </select>
  )
}
