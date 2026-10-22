import { useEffect } from 'react'
import { BellOff } from 'lucide-react'
import { api } from '../api'
import { t } from '../i18n'
import { useLoad } from '../state'

// Shown in the Mac app's window while macOS doesn't allow its notifications:
// no reminder would come. It checks again when the window comes back to the
// front and every half minute, so it goes away once they're turned on.
export function NotifyWarning() {
  const { data, reload } = useLoad(() => api.notifications().catch(() => ({ allowed: null })), [])
  const off = data?.allowed === false
  useEffect(() => {
    if (!off) return
    const id = setInterval(reload, 30_000)
    window.addEventListener('focus', reload)
    return () => {
      clearInterval(id)
      window.removeEventListener('focus', reload)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [off])
  if (!off) return null
  return (
    <div className="notice warn" style={{ marginBottom: 20 }}>
      <BellOff size={18} />
      <span style={{ flex: '1 1 220px' }}>{t('Mac 没有允许健康足迹发通知，吃药、预约和打卡提醒都不会弹出。')}</span>
      <button className="btn sm" onClick={() => api.openNotificationSettings()}>{t('打开通知设置')}</button>
    </div>
  )
}
