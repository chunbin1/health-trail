import { useEffect, useRef, useState, type PointerEvent as PE, type ReactNode } from 'react'
import { useBodyLock } from '../state'

export function useNarrow() {
  const [narrow, setNarrow] = useState(() => typeof window !== 'undefined' && window.matchMedia('(max-width: 760px)').matches)
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 760px)')
    const on = () => setNarrow(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
  return narrow
}

// A phone drawer that rises from the bottom edge and covers the tab bar.
// Tap the dimmed page, or pull the sheet down, to close. Pulling only starts
// from the handle, or from the list once it is already scrolled to the top,
// so a long list can still scroll.
// `keep` leaves the children mounted while the drawer is closed, so a chat
// can stay put between opens. `open` defaults to shown.
export function BottomSheet({ label, onClose, children, open = true, keep = false, className = '' }: {
  label?: string; onClose: () => void; children: ReactNode; open?: boolean; keep?: boolean; className?: string
}) {
  useBodyLock(open)
  const panel = useRef<HTMLDivElement>(null)
  const drag = useRef<{ y: number; pulling: boolean } | null>(null)
  const shiftRef = useRef(0)
  const [shift, setShift] = useState(0)
  const [settle, setSettle] = useState(false)
  useEffect(() => {
    if (!open) return
    shiftRef.current = 0
    setShift(0)
    setSettle(false)
  }, [open])
  if (!open && !keep) return null

  function place(n: number) {
    shiftRef.current = n
    setShift(n)
  }

  function down(e: PE<HTMLDivElement>) {
    const t = e.target as HTMLElement
    const fromGrab = !!t.closest('.sheet-grab')
    // typing and the tool buttons stay put; the handle and the page behind the list can pull
    if (!fromGrab && t.closest('input, textarea, button, a, select, label')) {
      drag.current = null
      return
    }
    drag.current = { y: e.clientY, pulling: fromGrab }
  }

  function move(e: PE<HTMLDivElement>) {
    const d = drag.current
    const el = panel.current
    if (!d || !el) return
    const dy = e.clientY - d.y
    if (!d.pulling) {
      const scroller = scrolledAncestor(e.target as HTMLElement, el)
      if (dy < 8 || (scroller && scroller.scrollTop > 0)) return
      d.pulling = true
    }
    if (!el.hasPointerCapture(e.pointerId)) el.setPointerCapture(e.pointerId)
    setSettle(false)
    place(Math.max(0, dy))
  }

  function up(e: PE<HTMLDivElement>) {
    const d = drag.current
    drag.current = null
    const el = panel.current
    if (el?.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId)
    if (!d?.pulling) return
    if (shiftRef.current > 96) {
      setSettle(true)
      place(el?.offsetHeight || 480)
      window.setTimeout(onClose, 180)
    } else {
      setSettle(true)
      place(0)
    }
  }

  return (
    <div className={'sheet-backdrop' + (open ? '' : ' sheet-off')} hidden={!open} onClick={onClose}>
      <div
        ref={panel}
        className={'sheet-panel' + (className ? ' ' + className : '') + (settle ? ' settle' : '') + (shift > 0 && !settle ? ' dragging' : '')}
        role="dialog"
        aria-label={label}
        style={{ transform: shift ? `translateY(${shift}px)` : undefined }}
        onClick={(e) => e.stopPropagation()}
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={up}
      >
        <div className="sheet-grab" />
        {children}
      </div>
    </div>
  )
}

function scrolledAncestor(node: HTMLElement | null, stop: HTMLElement) {
  let n = node
  while (n && n !== stop) {
    if (n.scrollHeight > n.clientHeight + 1) return n
    n = n.parentElement
  }
  return null
}
