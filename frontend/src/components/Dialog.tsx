import { useEffect, useRef, useState } from 'react'
import { t } from '../i18n'
import { Modal } from './Modal'

// confirm() and alert() do nothing in the Mac window: WKWebView shows them
// only if the app draws them, and Wails doesn't, so confirm() answers
// "cancel" at once and alert() is never seen. These draw them in the page,
// the same in the window, the browser and the phone.

interface Pending {
  message: string
  ok: string
  cancel: boolean // false for a notice: one button
  danger: boolean
  resolve: (ok: boolean) => void
}

let show: ((p: Pending) => void) | null = null

// ask resolves to true when the person chose `ok`.
export function ask(message: string, opts: { ok?: string; danger?: boolean } = {}): Promise<boolean> {
  return new Promise((resolve) => {
    const p = { message, ok: opts.ok ?? t('确定'), cancel: true, danger: !!opts.danger, resolve }
    if (show) show(p)
    else resolve(window.confirm(message))
  })
}

// tell shows a message, an error say, until the person closes it.
export function tell(message: string): Promise<void> {
  return new Promise((resolve) => {
    const p = { message, ok: t('好'), cancel: false, danger: false, resolve: () => resolve() }
    if (show) show(p)
    else { window.alert(message); resolve() }
  })
}

export function DialogHost() {
  const [queue, setQueue] = useState<Pending[]>([])
  const okRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    show = (p) => setQueue((q) => [...q, p])
    return () => { show = null }
  }, [])
  const cur = queue[0]
  // Return confirms, but not a deletion: that one has to be clicked
  useEffect(() => { if (cur && !cur.danger) okRef.current?.focus() }, [cur])
  if (!cur) return null
  const done = (ok: boolean) => {
    cur.resolve(ok)
    setQueue((q) => q.slice(1))
  }
  return (
    <Modal title={cur.cancel ? t('请确认') : t('提示')} onClose={() => done(false)}>
      <div className="modal-body" style={{ whiteSpace: 'pre-line' }}>{cur.message}</div>
      <div className="modal-foot">
        <span className="spacer" />
        {cur.cancel && <button type="button" className="btn" onClick={() => done(false)}>{t('取消')}</button>}
        <button type="button" ref={okRef} className={'btn ' + (cur.danger ? 'danger' : 'primary')} onClick={() => done(true)}>{cur.ok}</button>
      </div>
    </Modal>
  )
}
