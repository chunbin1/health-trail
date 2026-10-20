import { useEffect, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { t } from '../i18n'
import { IconButton } from '../ui'

function useEscape(onClose: () => void) {
  useEffect(() => {
    const on = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', on)
    return () => window.removeEventListener('keydown', on)
  }, [onClose])
}

// A dialog. Its children lay themselves out in .modal-body and .modal-foot,
// so a form can keep its buttons inside the <form>.
export function Modal({ title, icon, onClose, children, wide }: { title: string; icon?: ReactNode; onClose: () => void; children: ReactNode; wide?: boolean }) {
  useEscape(onClose)
  return (
    <div className="overlay center" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={'modal' + (wide ? ' wide' : '')} role="dialog" aria-label={title}>
        <div className="modal-head">
          {icon}
          <h2>{title}</h2>
          <IconButton label={t('关闭')} onClick={onClose}><X size={18} /></IconButton>
        </div>
        {children}
      </div>
    </div>
  )
}

export function Drawer({ onClose, head, children }: { onClose: () => void; head?: ReactNode; children: ReactNode }) {
  useEscape(onClose)
  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="drawer" role="dialog">
        <div className="drawer-head">
          {head}
          <span className="spacer" />
          <IconButton label={t('关闭')} onClick={onClose}><X size={18} /></IconButton>
        </div>
        <div className="drawer-body">{children}</div>
      </div>
    </div>
  )
}
