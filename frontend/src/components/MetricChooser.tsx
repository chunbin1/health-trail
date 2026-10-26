import { t, tx } from '../i18n'
import { KindIcon, Switch } from '../ui'
import { Modal } from './Modal'

// MetricChooser turns body metrics on and off for a page (今天, 身体数据):
// one switch each, on for shown. Each page keeps its own choice.
export function MetricChooser({ title, names, shown, onChange, note, onClose }: {
  title: string
  names: string[]
  shown: (name: string) => boolean
  onChange: (name: string, on: boolean) => void
  note: string
  onClose: () => void
}) {
  return (
    <Modal title={title} icon={<KindIcon kind="metrics" />} onClose={onClose}>
      <div className="modal-body">
        {names.map((m) => (
          <div key={m} className="row" style={{ gap: 8, flexWrap: 'nowrap', padding: '8px 0' }}>
            <div style={{ flex: 1, minWidth: 0 }}>{tx(m)}</div>
            <Switch checked={shown(m)} label={tx(m)} onChange={(v) => onChange(m, v)} />
          </div>
        ))}
        <div className="small muted" style={{ marginTop: 8 }}>{note}</div>
      </div>
      <div className="modal-foot"><span className="spacer" /><button className="btn primary" onClick={onClose}>{t('完成')}</button></div>
    </Modal>
  )
}
