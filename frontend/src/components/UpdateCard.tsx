import { useEffect, useState } from 'react'
import { ArrowUpCircle, X } from 'lucide-react'
import { api, type UpdateStatus } from '../api'
import { t } from '../i18n'

// The version the card was closed for; it comes back with a newer one, or
// when the app opens again.
let closedFor = ''

// The card at the bottom of the sidebar when the Mac app has found a newer
// version, like Claude's own: one click backs up (when backups are set up),
// downloads, checks the signature and restarts, with how it goes shown in
// place. The release notes and the "are you sure" stay in Settings.
export function UpdateCard({ status }: { status: UpdateStatus | null }) {
  const [cur, setCur] = useState<UpdateStatus | null>(null)
  const [err, setErr] = useState('')
  const [, setClosed] = useState('')
  // a newer status from the app replaces what this card last heard,
  // including a request it had turned down
  useEffect(() => {
    setCur(null)
    setErr('')
  }, [status])
  const s = cur || status
  const st = s?.state
  // a check under way doesn't hold up the button: installing goes ahead
  const running = !!st?.phase
  useEffect(() => {
    if (!running) return
    const id = setInterval(() => api.updateStatus().then(setCur, () => {}), 700)
    return () => clearInterval(id)
  }, [running])
  const r = st?.latest
  if (!s || !st || !r || (closedFor === r.version && !st.phase)) return null

  const canInstall = !!s.installable && !s.why_not
  async function install() {
    if (running) return
    setErr('')
    setCur({ ...s!, state: { ...st!, phase: 'backup', error: '', install_error: '' } })
    try {
      setCur(await api.installUpdate())
      // Settings, if it is open, follows along
      window.dispatchEvent(new Event('update-checked'))
    } catch (e: any) {
      setCur(null)
      setErr(e.message)
    }
  }
  const pct = st.total ? Math.floor(((st.done || 0) * 100) / st.total) : 0
  const phase = st.phase === 'backup' ? t('正在备份…') : st.phase === 'download' ? (st.total ? t('正在下载 {pct}%', { pct }) : t('正在下载…')) : st.phase === 'restart' ? t('正在重启…') : ''
  // only why an install didn't finish: a check that couldn't reach GitHub
  // leaves the version found, and isn't an update that failed
  const error = err || st.install_error
  return (
    <div className="update-card" role="status">
      <div className="update-card-head">
        <ArrowUpCircle size={16} />
        <div className="update-card-title">
          <b>{t('有新版本')}</b>
          <span className="small muted">{r.version} · <a href="#/settings">{t('更新说明')}</a></span>
        </div>
        {!phase && (
          <button className="update-card-close" aria-label={t('关闭')} title={t('关闭')}
            onClick={() => { closedFor = r.version; setClosed(r.version) }}>
            <X size={14} />
          </button>
        )}
      </div>
      {phase ? (
        <div className="update-progress">
          <div className="small">{phase}</div>
          <div className="bar"><span style={{ width: (st.phase === 'download' ? pct : st.phase === 'restart' ? 100 : 0) + '%' }} /></div>
        </div>
      ) : (
        <>
          {error && <div className="small update-card-error">{error}</div>}
          {canInstall
            ? <button className="btn primary sm" onClick={install}>{t('更新并重启')}</button>
            : <button className="btn primary sm" onClick={() => api.openReleasePage()}>{t('打开下载页面')}</button>}
        </>
      )}
    </div>
  )
}
