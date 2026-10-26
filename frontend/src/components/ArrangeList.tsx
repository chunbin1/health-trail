import { useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react'
import { GripVertical } from 'lucide-react'
import { t } from '../i18n'
import { Switch } from '../ui'
import { Modal } from './Modal'

export interface ArrangeItem {
  key: string
  label: string
}

// ArrangeList lets the person put a list in order (drag the handle, or the
// arrow keys on it) and turn items on and off, in a dialog. It reports the
// whole choice on each change: the keys in order and the ones turned off.
// Where the choice is kept is the caller's (useListChoice in state.tsx).
export function ArrangeList({ title, icon, items, shown, onChange, note, onClose }: {
  title: string
  icon?: ReactNode
  items: ArrangeItem[] // in the current order
  shown: (key: string) => boolean
  onChange: (order: string[], hidden: string[]) => void
  note?: string
  onClose: () => void
}) {
  const [order, setOrder] = useState(() => items.map((i) => i.key))
  const label = new Map(items.map((i) => [i.key, i.label]))
  const keys = order.filter((k) => label.has(k))
  const drag = useRef<{ key: string; startY: number; from: number; rowH: number; before: string } | null>(null)
  const [dragging, setDragging] = useState<string | null>(null)
  const [offset, setOffset] = useState(0)
  const latest = useRef(keys)
  latest.current = keys

  const report = (next: string[], hiddenNow = (k: string) => !shown(k)) => onChange(next, next.filter(hiddenNow))

  function down(e: PointerEvent<HTMLButtonElement>, key: string) {
    if (e.button !== 0) return
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    const row = e.currentTarget.closest('.arrange-row')!.getBoundingClientRect()
    drag.current = { key, startY: e.clientY, from: keys.indexOf(key), rowH: row.height, before: keys.join('\n') }
    setDragging(key)
    setOffset(0)
  }
  function move(e: PointerEvent<HTMLButtonElement>) {
    const d = drag.current
    if (!d) return
    const dy = e.clientY - d.startY
    const to = Math.max(0, Math.min(keys.length - 1, d.from + Math.round(dy / d.rowH)))
    if (keys.indexOf(d.key) !== to) {
      const next = keys.filter((k) => k !== d.key)
      next.splice(to, 0, d.key)
      setOrder(next)
    }
    // the row follows the pointer between the places it snaps to
    setOffset(dy - (to - d.from) * d.rowH)
  }
  function up() {
    const d = drag.current
    if (!d) return
    drag.current = null
    setDragging(null)
    setOffset(0)
    if (latest.current.join('\n') !== d.before) report(latest.current)
  }
  function key(e: KeyboardEvent<HTMLButtonElement>, k: string) {
    const by = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0
    const i = keys.indexOf(k)
    if (!by || i + by < 0 || i + by >= keys.length) return
    e.preventDefault()
    const next = [...keys]
    next.splice(i, 1)
    next.splice(i + by, 0, k)
    setOrder(next)
    report(next)
  }

  return (
    <Modal title={title} icon={icon} onClose={onClose}>
      <div className="modal-body">
        <div className="arrange">
          {keys.map((k) => (
            <div key={k} className={'arrange-row' + (dragging === k ? ' dragging' : '') + (shown(k) ? '' : ' off')}
              style={dragging === k ? { transform: `translateY(${offset}px)` } : undefined}>
              <button type="button" className="arrange-grip" aria-label={t('拖动调整「{name}」的顺序', { name: label.get(k)! })}
                title={t('拖动调整顺序')}
                onPointerDown={(e) => down(e, k)} onPointerMove={move} onPointerUp={up} onPointerCancel={up}
                onKeyDown={(e) => key(e, k)}>
                <GripVertical size={16} />
              </button>
              <div className="arrange-label">{label.get(k)}</div>
              <Switch checked={shown(k)} label={label.get(k)}
                onChange={(v) => report(keys, (x) => (x === k ? !v : !shown(x)))} />
            </div>
          ))}
        </div>
        {note && <div className="small muted" style={{ marginTop: 10 }}>{note}</div>}
      </div>
      <div className="modal-foot"><span className="spacer" /><button className="btn primary" onClick={onClose}>{t('完成')}</button></div>
    </Modal>
  )
}
