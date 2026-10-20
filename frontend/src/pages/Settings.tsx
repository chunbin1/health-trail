import { useEffect, useState } from 'react'
import { Database, Languages, Smartphone } from 'lucide-react'
import { api } from '../api'
import { t, type Lang } from '../i18n'
import { useApp, useLoad } from '../state'
import { Switch } from '../ui'
import { BackupSettings } from './Backups'
import { CodeBox, Section } from './Connect'

// Each language named in itself, as language pickers do.
const LANGS: { id: Lang; name: string }[] = [{ id: 'zh', name: '中文' }, { id: 'en', name: 'English' }]

// Settings: the language, backups, reaching the app from a phone, and where
// the data lives.
export function Settings() {
  const { lang, switchLang } = useApp()
  const { data: info, error, reload } = useLoad(() => api.info(), [])
  // switching restarts the local server, which takes a couple of seconds:
  // the switch shows the new state at once, and takes no clicks meanwhile
  const [pending, setPending] = useState<boolean | null>(null)
  // until the reloaded info agrees, so the switch never flicks back
  useEffect(() => {
    if (pending !== null && info?.lan === pending) setPending(null)
  }, [info, pending])
  if (error) return <div className="error">{error}</div>
  if (!info) return null
  const lan = pending ?? info.lan

  async function toggleLAN(on: boolean) {
    if (pending !== null) return
    setPending(on)
    try {
      await api.setLAN(on)
      reload()
    } catch (e: any) {
      setPending(null)
      alert(e.message)
    }
  }

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

        <Section icon={<Smartphone size={20} />} title={t('手机访问')}
          sub={t('同一 Wi-Fi 或 Tailscale 网络下的手机，用浏览器打开链接即可记录和打卡；在 Safari 里「添加到主屏幕」后就像一个 App。')}
          right={<div className="row" style={{ gap: 8 }}>
            {pending !== null && <span className="small muted">{t('正在切换…')}</span>}
            <Switch checked={lan} onChange={toggleLAN} label={t('允许手机访问')} />
          </div>}>
          {info.listen_error && <div className="error">{t('本地服务没有启动：{error}', { error: info.listen_error })}</div>}
          {info.lan && pending === null && (info.phone_urls?.length
            ? <div className="stack" style={{ gap: 8 }}>{info.phone_urls.map((u) => <CodeBox key={u} text={u} />)}<div className="small muted">{t('链接里带着访问令牌，只发给自己，不要分享给别人。')}</div></div>
            : <div className="sub">{t('没有找到局域网地址，请检查网络连接。')}</div>)}
          {!lan && <div className="small muted">{t('手机上的 AI App（如 Claude、ChatGPT）只能连接公网服务器，这部分放在第二期（需要隧道和登录授权）。')}</div>}
        </Section>

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
