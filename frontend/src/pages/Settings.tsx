import { useState } from 'react'
import { Database, Languages, Power } from 'lucide-react'
import { api } from '../api'
import { t, type Lang } from '../i18n'
import { useApp, useLoad } from '../state'
import { Switch } from '../ui'
import { NotifyWarning } from '../components/NotifyWarning'
import { BackupSettings } from './Backups'
import { Section } from './Connect'

// Each language named in itself, as language pickers do.
const LANGS: { id: Lang; name: string }[] = [{ id: 'zh', name: '中文' }, { id: 'en', name: 'English' }]

// Opening at login, in the Mac app only (elsewhere the API isn't there and
// the section isn't shown). The switch moves at once; the login item is
// read back after.
function AtLogin() {
  const { data, error } = useLoad(() => api.autostart(), [])
  const [on, setOn] = useState<boolean | null>(null)
  const [busy, setBusy] = useState(false)
  if (error || !data) return null
  const checked = on ?? data.enabled
  async function change(v: boolean) {
    if (busy) return
    setBusy(true)
    setOn(v)
    try {
      setOn((await api.setAutostart(v)).enabled)
    } catch (e: any) {
      setOn(!v)
      alert(e.message)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Section icon={<Power size={20} />} title={t('开机自动启动')}
      sub={t('登录 Mac 后在菜单栏运行，不打开窗口，到点照常提醒。关掉窗口也会留在菜单栏；要完全退出，点菜单栏的图标 →「退出健康足迹」。')}
      right={<Switch checked={checked} onChange={change} label={t('开机自动启动')} />} />
  )
}

// Settings: the language, backups, and where the data lives. (Phone access
// is hidden until it's reworked.)
export function Settings() {
  const { lang, switchLang } = useApp()
  const { data: info, error } = useLoad(() => api.info(), [])
  if (error) return <div className="error">{error}</div>
  if (!info) return null

  return (
    <>
      <div className="page-head">
        <div className="titles"><h1>{t('设置')}</h1></div>
      </div>
      <div className="stack" style={{ maxWidth: 760 }}>
        <NotifyWarning />
        <Section icon={<Languages size={20} />} title={t('语言')}
          right={
            <select className="input" style={{ width: 'auto' }} aria-label={t('语言')} value={lang}
              onChange={(e) => switchLang(e.target.value as Lang)}>
              {LANGS.map((l) => <option key={l.id} value={l.id} lang={l.id === 'en' ? 'en' : 'zh-CN'}>{l.name}</option>)}
            </select>
          } />

        <AtLogin />

        <BackupSettings />

        <Section icon={<Database size={20} />} title={t('数据')} sub={t('数据库是 health.db（SQLite），附件在 files/。备份时复制整个目录即可；建议开启 FileVault 磁盘加密。')}>
          <dl className="kv" style={{ boxShadow: 'none', padding: 0, background: 'none' }}>
            <div className="kv-row"><dt>{t('数据目录')}</dt><dd><code>{info.data_dir}</code></dd></div>
            <div className="kv-row"><dt>{t('本地服务')}</dt><dd>{info.listening || t('未启动')}</dd></div>
            <div className="kv-row"><dt>{t('版本')}</dt><dd>{info.version}</dd></div>
          </dl>
        </Section>
      </div>
    </>
  )
}
