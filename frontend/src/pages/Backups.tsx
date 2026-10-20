import { useEffect, useState, type ReactNode } from 'react'
import { ArchiveRestore, Cloud, FolderOpen } from 'lucide-react'
import { api, fmtDate, type BackupConfig, type BackupState } from '../api'
import { DateTimeInput } from '../components/DateTimeInput'
import { t } from '../i18n'
import { useApp, useLoad } from '../state'
import { Empty, Switch } from '../ui'

const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'] as const

const size = (n: number) => (n < 1024 * 1024 ? `${Math.max(1, Math.round(n / 1024))} KB` : `${(n / 1024 / 1024).toFixed(1)} MB`)

// Backups, as cards on the settings page: where they go, what makes one, and
// the ones there are, each of which can be restored. Changes save at once.
export function BackupSettings() {
  const { changed, toast } = useApp()
  const { data, error, reload } = useLoad(() => api.backup(), [])
  const [cfg, setCfg] = useState<BackupConfig>()
  const [dir, setDir] = useState('')
  const [problem, setProblem] = useState('')
  const [busy, setBusy] = useState<'' | 'run' | string>('')

  useEffect(() => {
    if (data) {
      setCfg(data.config)
      setDir(data.config.dir)
    }
  }, [data])

  if (error) return <div className="error">{error}</div>
  if (!data || !cfg) return null

  async function save(next: BackupConfig) {
    setCfg(next)
    try {
      const saved = await api.saveBackup(next)
      setCfg(saved)
      setDir(saved.dir)
      setProblem('')
      reload()
    } catch (e: any) {
      setProblem(e.message)
    }
  }
  const set = (patch: Partial<BackupConfig>) => save({ ...cfg!, ...patch })
  const saveDir = (d: string) => d.trim() !== cfg!.dir && set({ dir: d.trim() })

  async function choose() {
    try {
      const { dir: d } = await api.chooseBackupDir()
      if (d) set({ dir: d })
    } catch (e: any) {
      setProblem(e.message)
    }
  }

  async function runNow() {
    setBusy('run')
    try {
      const { name } = await api.runBackup()
      toast(t('已备份：{name}', { name }))
    } catch (e: any) {
      toast(e.message)
    } finally {
      setBusy('')
      reload()
    }
  }

  async function restore(name: string, time: string) {
    if (!confirm(t('恢复到 {time} 的备份？\n\n恢复前会先把当前数据另存一份（文件名以 before-restore 结尾），恢复错了可以从那份再恢复回来。', { time: fmtDate(time.slice(0, 16)) }))) return
    setBusy(name)
    try {
      await api.restoreBackup(name)
      toast(t('已恢复到 {time} 的数据', { time: fmtDate(time.slice(0, 16)) }))
      changed()
    } catch (e: any) {
      alert(e.message)
      reload()
    } finally {
      setBusy('')
    }
  }

  return (
    <>
        <section className="card">
          <div className="card-head"><h2>{t('备份位置')}</h2></div>
          <div className="small muted" style={{ marginBottom: 10 }}>{t('把数据库和附件打包成一个 zip 文件，存到你选的文件夹')}</div>
          <div className="row" style={{ flexWrap: 'nowrap' }}>
            <input className="input" style={{ flex: 1, minWidth: 0 }} value={dir} placeholder={t('选择或填写一个文件夹，如 /Users/你/Documents/HealthVaultBackups')}
              onChange={(e) => setDir(e.target.value)} onBlur={() => saveDir(dir)} onKeyDown={(e) => e.key === 'Enter' && saveDir(dir)} />
            {data.can_choose_dir && <button className="btn" onClick={choose}><FolderOpen size={15} />{t('选择…')}</button>}
          </div>
          {data.suggested_dir && cfg.dir !== data.suggested_dir && (
            <button className="btn sm ghost" style={{ marginTop: 8 }} onClick={() => set({ dir: data.suggested_dir })}><Cloud size={14} />{t('用 iCloud 云盘')}</button>
          )}
          {problem && <div className="error" style={{ marginTop: 10 }}>{problem}</div>}
          <div className="small muted" style={{ marginTop: 10 }}>{t('建议放在 iCloud 云盘、移动硬盘或网盘的同步文件夹里，别和数据放在同一块硬盘上。')}</div>
        </section>

        <section className="card">
          <div className="card-head"><h2>{t('自动备份')}</h2></div>
          <div className="list">
            <Row label={t('每天')} on={cfg.daily} onChange={(daily) => set({ daily })}>
              <DateTimeInput mode="time" required value={cfg.daily_at} onChange={(daily_at) => set({ daily_at })} />
            </Row>
            <Row label={t('每周')} on={cfg.weekly} onChange={(weekly) => set({ weekly })}>
              <select className="input" style={{ width: 'auto' }} value={cfg.weekday} onChange={(e) => set({ weekday: Number(e.target.value) })}>
                {WEEKDAYS.map((d, i) => <option key={d} value={i}>{t(d)}</option>)}
              </select>
              <DateTimeInput mode="time" required value={cfg.weekly_at} onChange={(weekly_at) => set({ weekly_at })} />
            </Row>
            <Row label={t('有改动后')} hint={t('记录或修改后，等 10 分钟没有新改动就备份一次')} on={cfg.on_change} onChange={(on_change) => set({ on_change })} />
            <Row label={t('退出 App 时')} on={cfg.on_quit} onChange={(on_quit) => set({ on_quit })} />
            <div className="row" style={{ padding: '10px 0', gap: 8 }}>
              <span style={{ flex: 1 }}>{t('最多保留')}</span>
              <input className="input num" style={{ width: 72 }} inputMode="numeric" defaultValue={cfg.keep} key={cfg.keep}
                onBlur={(e) => Number(e.target.value) !== cfg.keep && set({ keep: Number(e.target.value) })} />
              <span className="muted">{t('份，更早的自动删除')}</span>
            </div>
          </div>
          <div className="small muted" style={{ marginTop: 8 }}>{t('App 开着的时候才会自动备份；错过的时间会在下次打开时补上。')}</div>
        </section>

        <section className="card">
          <div className="card-head">
            <h2>{t('备份记录')}</h2>
            <button className="btn primary sm" disabled={!cfg.dir || !!busy} onClick={runNow}>{busy === 'run' ? t('备份中…') : t('立即备份')}</button>
          </div>
          <Status s={data} />
          {data.backups.length === 0 ? <Empty compact>{cfg.dir ? t('这个文件夹里还没有备份') : t('先在上面选好备份位置')}</Empty> : (
            <div className="list">
              {data.backups.map((b) => (
                <div key={b.name} className="row-item no-ico" style={{ cursor: 'default' }}>
                  <div className="body">
                    <div className="title">{fmtDate(b.time.slice(0, 16))}{(b.name.includes('-before-restore') || b.name.includes('-恢复前')) && <span className="tag" style={{ marginLeft: 8 }}>{t('恢复前')}</span>}</div>
                    <div className="meta ellipsis">{b.name} · {size(b.size)}</div>
                  </div>
                  <button className="btn sm" disabled={!!busy} onClick={() => restore(b.name, b.time)}><ArchiveRestore size={14} />{busy === b.name ? t('恢复中…') : t('恢复')}</button>
                </div>
              ))}
            </div>
          )}
        </section>
    </>
  )
}

// A trigger: its switch always at the right end of the first line, so the
// switches line up at any width; its settings (times) on a line below.
function Row({ label, hint, on, onChange, children }: { label: string; hint?: string; on: boolean; onChange: (v: boolean) => void; children?: ReactNode }) {
  return (
    <div style={{ padding: '10px 0' }}>
      <div className="row" style={{ gap: 8, flexWrap: 'nowrap' }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div>{label}</div>
          {hint && <div className="small muted">{hint}</div>}
        </div>
        <Switch checked={on} onChange={onChange} label={label} />
      </div>
      {children && <div className="row" style={{ gap: 8, marginTop: 8, opacity: on ? 1 : 0.5 }}>{children}</div>}
    </div>
  )
}

function Status({ s }: { s: BackupState }) {
  const { last_at, last_file, last_error, error_at } = s.status
  if (last_error && (!last_at || (error_at || '') > last_at))
    return <div className="error" style={{ marginBottom: 12 }}>{t('上次备份失败（{time}）：{error}', { time: fmtDate((error_at || '').slice(0, 16)), error: last_error })}</div>
  if (last_at)
    return <div className="small muted" style={{ marginBottom: 12 }}>{t('上次备份：{time} · {file}', { time: fmtDate(last_at.slice(0, 16)), file: last_file || '' })}</div>
  return null
}
