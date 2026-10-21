import { Database, Languages } from 'lucide-react'
import { api } from '../api'
import { t, type Lang } from '../i18n'
import { useApp, useLoad } from '../state'
import { BackupSettings } from './Backups'
import { Section } from './Connect'

// Each language named in itself, as language pickers do.
const LANGS: { id: Lang; name: string }[] = [{ id: 'zh', name: '中文' }, { id: 'en', name: 'English' }]

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
        <Section icon={<Languages size={20} />} title={t('语言')}
          right={
            <select className="input" style={{ width: 'auto' }} aria-label={t('语言')} value={lang}
              onChange={(e) => switchLang(e.target.value as Lang)}>
              {LANGS.map((l) => <option key={l.id} value={l.id} lang={l.id === 'en' ? 'en' : 'zh-CN'}>{l.name}</option>)}
            </select>
          } />

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
