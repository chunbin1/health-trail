import { useEffect, useState } from 'react'
import { Database, FolderOpen, Languages, Power, RefreshCw, Smartphone } from 'lucide-react'
import { api, fmtDate, type CalSync, type UpdateStatus } from '../api'
import { t, type Lang } from '../i18n'
import { useApp, useLoad } from '../state'
import { Switch } from '../ui'
import { NotifyWarning } from '../components/NotifyWarning'
import { ask, tell } from '../components/Dialog'
import { BackupSettings } from './Backups'
import { Section } from './Connect'

// Each language named in itself, as language pickers do.
const LANGS: { id: Lang; name: string }[] = [{ id: 'zh', name: '中文' }, { id: 'en', name: 'English' }]

// Appointments into Calendar and medicines into Reminders, which iCloud takes
// to the iPhone and the watch. Mac app only, like AtLogin. Turning one on
// may wait on the system's permission prompt, so the switch says so.
function PhoneSync() {
  const { data, error, reload } = useLoad(() => api.calsync(), [])
  const [cur, setCur] = useState<CalSync | null>(null)
  const [busy, setBusy] = useState<'events' | 'reminders' | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    // the last sync and its errors, as they come
    const id = setInterval(reload, 15_000)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const s = cur && !busy ? { ...data!, ...cur, last_at: data!.last_at, last_error: data!.last_error } : cur || data
  if (error || !s || !s.title) return null

  async function change(kind: 'events' | 'reminders', on: boolean) {
    if (busy) return
    setBusy(kind)
    setErr('')
    setCur({ ...s!, [kind]: on })
    try {
      setCur(await api.setCalsync({ [kind]: on }))
      reload()
    } catch (e: any) {
      setCur({ ...s!, [kind]: !on })
      setErr(e.message)
    } finally {
      setBusy(null)
    }
  }

  const row = (kind: 'events' | 'reminders', label: string, hint: string) => {
    // on without full access: ask again, or, once refused, the way to the settings
    const acc = s[kind === 'events' ? 'events_access' : 'reminders_access']
    const denied = acc === 'denied'
    const ask = s[kind] && !denied && acc !== 'full'
    return (
      <div className="sync-row">
        <div style={{ flex: 1, minWidth: 0 }}>
          <div>{label}</div>
          <div className="small muted">{busy === kind && s[kind] ? t('请在系统弹窗里点「允许」…') : hint}</div>
          {denied && (
            <button className="btn sm" style={{ marginTop: 6 }} onClick={() => api.openCalPrivacy(kind === 'reminders')}>
              {t('打开隐私设置')}
            </button>
          )}
          {ask && busy !== kind && (
            <button className="btn sm" style={{ marginTop: 6 }} onClick={() => change(kind, true)}>{t('允许访问')}</button>
          )}
        </div>
        <Switch checked={!!s[kind]} onChange={(v) => change(kind, v)} label={label} />
      </div>
    )
  }

  return (
    <Section icon={<Smartphone size={20} />} title={t('同步到 iPhone')}
      sub={t('写进 Mac 的「日历」和「提醒事项」里叫「{title}」的日历和列表，iCloud 会同步到 iPhone 和手表，在那边也会提醒。', { title: s.title })}>
      {row('events', t('预约写进「日历」'), t('前一天晚上 8 点和开始前提醒'))}
      {row('reminders', t('吃药写进「提醒事项」'), t('在 iPhone 上勾掉，这里就记一次吃药；这里记了，那边也会勾掉'))}
      {(s.events || s.reminders) && s.last_at && (
        <div className="small muted" style={{ marginTop: 10 }}>
          {t('上次同步 {time}', { time: fmtDate(s.last_at.slice(0, 16)) })}{s.source ? ' · ' + s.source : ''}
        </div>
      )}
      {(err || s.last_error) && <div className="error" style={{ marginTop: 10 }}>{err || s.last_error}</div>}
    </Section>
  )
}

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
      tell(e.message)
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

// Looking for a newer release and installing it, in the Mac app only. The
// check button keeps its place whatever is found; while an install runs the
// page asks how it goes, until the app quits and reopens as the new version.
function Updates() {
  const { data, error } = useLoad(() => api.updateStatus(), [])
  const [cur, setCur] = useState<UpdateStatus | null>(null)
  const [err, setErr] = useState('')
  const s = cur || data
  const st = s?.state
  const running = !!st && (st.checking || !!st.phase)
  useEffect(() => {
    if (!running) return
    const id = setInterval(() => api.updateStatus().then(setCur, () => {}), 700)
    return () => clearInterval(id)
  }, [running])
  if (error || !s || !st) return null

  async function run(f: () => Promise<UpdateStatus>) {
    setErr('')
    try {
      setCur(await f())
    } catch (e: any) {
      setErr(e.message)
    }
  }
  async function check() {
    if (running) return
    setCur({ ...s!, state: { ...st!, checking: true, error: '' } })
    await run(api.checkUpdate)
  }
  async function install() {
    const r = st!.latest
    if (running || !r || !(await ask(t('更新到 {version}？\n\n设置了备份的话，会先备份一次数据；然后下载新版本、检查签名，替换后重新打开健康足迹。', { version: r.version }), { ok: t('更新并重启') }))) return
    setCur({ ...s!, state: { ...st!, phase: 'backup', error: '' } })
    await run(api.installUpdate)
  }

  const r = st.latest
  const pct = st.total ? Math.floor(((st.done || 0) * 100) / st.total) : 0
  const phase = st.phase === 'backup' ? t('正在备份…') : st.phase === 'download' ? (st.total ? t('正在下载 {pct}%', { pct }) : t('正在下载…')) : st.phase === 'restart' ? t('正在重启…') : ''
  const canInstall = !!r && s.installable && !s.why_not
  return (
    <Section icon={<RefreshCw size={20} />} title={t('软件更新')}
      sub={t('当前版本 {version}', { version: s.current }) + (!r && st.checked_at && !st.error && !st.checking ? ' · ' + t('已是最新版本 · 上次检查 {time}', { time: fmtDate(st.checked_at.slice(0, 16)) }) : '')}
      right={<button className="btn sm" disabled={running} onClick={check}>{st.checking ? t('正在检查…') : t('检查更新')}</button>}>
      {r && (
        <div className="update-box">
          <div className="update-head">
            <b>{t('新版本 {version}', { version: r.version })}</b>
            {r.published && <span className="small muted">{t('发布于 {date}', { date: fmtDate(r.published.slice(0, 10)) })}</span>}
          </div>
          {r.notes && <div className="update-notes small">{r.notes}</div>}
          {phase ? (
            <div className="update-progress">
              <div className="small">{phase}</div>
              <div className="bar"><span style={{ width: (st.phase === 'download' ? pct : st.phase === 'restart' ? 100 : 0) + '%' }} /></div>
            </div>
          ) : (
            <div className="update-actions">
              {canInstall
                ? <button className="btn primary sm" onClick={install}>{t('更新并重启')}</button>
                : <button className="btn primary sm" onClick={() => api.openReleasePage()}>{t('打开下载页面')}</button>}
              {canInstall && <button className="btn sm" onClick={() => api.openReleasePage()}>{t('发布页面')}</button>}
            </div>
          )}
          {s.why_not && s.installable && !phase && <div className="small muted" style={{ marginTop: 8 }}>{s.why_not}</div>}
        </div>
      )}
      {(err || st.error) && <div className="error" style={{ marginBottom: 12 }}>{err || st.error}</div>}
      <div className="sync-row">
        <div style={{ flex: 1, minWidth: 0 }}>
          <div>{t('自动检查更新')}</div>
          <div className="small muted">{t('每隔几个小时、打开窗口时向 GitHub 查有没有新版本，只问版本号，不会发送任何健康数据。')}</div>
        </div>
        <Switch checked={s.auto} onChange={(v) => { setCur({ ...s, auto: v }); run(() => api.setAutoUpdate(v)) }} label={t('自动检查更新')} />
      </div>
    </Section>
  )
}

// Folders Claude may attach files from, in the App Store build only: its
// sandbox allows Downloads and nothing else outside the app, so a report on
// the Desktop needs its folder granted here once. The open dialog is what
// grants it.
function AllowedFolders() {
  const { data, error } = useLoad(() => api.folders(), [])
  const [list, setList] = useState<string[] | null>(null)
  const [busy, setBusy] = useState('')
  if (error || !data || !data.sandboxed) return null
  const folders = list ?? data.folders
  async function run(key: string, f: () => Promise<{ folders: string[] }>) {
    if (busy) return
    setBusy(key)
    try {
      setList((await f()).folders)
    } catch (e: any) {
      tell(e.message)
    } finally {
      setBusy('')
    }
  }
  async function remove(p: string) {
    if (busy || !(await ask(t('不再允许读取这个文件夹？\n\n{path}\n\nClaude 之后就存不了这里的文件；已经连着的 Claude 要重新连接一次。', { path: p }), { ok: t('移除'), danger: true }))) return
    run(p, () => api.removeFolder(p))
  }
  return (
    <Section icon={<FolderOpen size={20} />} title={t('允许读取的文件夹')}
      sub={t('App Store 版受系统沙盒限制，Claude 只能把「下载」里的文件存成附件。在这里添加别的文件夹（比如桌面、文稿），Claude 就能存那里的文件。添加或移除后，已经连着的 Claude 要重新连接一次。')}
      right={<button className="btn sm" disabled={!!busy} onClick={() => run('add', api.addFolder)}>{busy === 'add' ? t('等待选择…') : t('添加…')}</button>}>
      {folders.length > 0 && (
        <div className="list">
          {folders.map((p) => (
            <div key={p} className="row-item no-ico" style={{ cursor: 'default' }}>
              <div className="body"><div className="title ellipsis"><code>{p}</code></div></div>
              <button className="btn sm" disabled={!!busy} onClick={() => remove(p)}>{busy === p ? t('移除中…') : t('移除')}</button>
            </div>
          ))}
        </div>
      )}
    </Section>
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

        <Updates />

        <PhoneSync />

        <AllowedFolders />

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
