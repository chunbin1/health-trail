import { useEffect, useState, type CSSProperties, type FormEvent } from 'react'
import { Clock, Coffee, Droplets, Pause, Play, RotateCcw, SkipForward, Volume2 } from 'lucide-react'
import type { BreakTimerConfig } from '../api'
import { BreakRing } from '../components/BreakRing'
import { t } from '../i18n'
import { isIOSApp } from '../ios'
import { Switch } from '../ui'
import { playChime } from '../utils/chime'
import { useBreakTimer } from '../utils/useBreakTimer'

function shortDay(date: string): string {
  const parts = date.split('-')
  if (parts.length !== 3) return date
  return `${Number(parts[1])}/${Number(parts[2])}`
}

export function Breaks() {
  const { snap, remainingSec, error, busy, act, saveConfig } = useBreakTimer()
  const [draft, setDraft] = useState<BreakTimerConfig | null>(null)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [savedNotice, setSavedNotice] = useState(false)
  const [cfgError, setCfgError] = useState('')

  useEffect(() => {
    if (snap && !dirty) {
      setDraft(snap.config)
    }
  }, [snap, dirty])

  if (!snap || !draft) {
    return (
      <>
        <div className="page-head">
          <div className="titles">
            <h1>{t('休息')}</h1>
            <div className="sub">{t('定时提醒喝水和起身活动，久坐工作不伤身')}</div>
          </div>
        </div>
        {error ? <div className="error">{error}</div> : <div className="muted">{t('正在读取…')}</div>}
      </>
    )
  }

  const ios = isIOSApp()
  const { state, total_sec, today, history, config } = snap

  const statusText = (() => {
    switch (state) {
      case 'working': return t('专注工作中')
      case 'break': return t('正在休息')
      case 'break_done': return t('休息结束')
      case 'paused': return t('已暂停')
      case 'outside_hours': return t('非工作时段')
      default: return t('未开启')
    }
  })()

  const phaseHint = (() => {
    switch (state) {
      case 'working':
        return t('工作 {m} 分钟后将提醒你喝水并起身活动。', { m: config.work_minutes })
      case 'break':
        return t('离开屏幕站一会儿，喝口水放松眼睛和肩颈。')
      case 'break_done':
        return t('本次休息已满，回到电脑前再手动开始下一轮。')
      case 'paused':
        return t('计时已暂停，点击「继续计时」即可接着倒计时。')
      case 'outside_hours':
        return t('当前不在设定的工作时段（{start}–{end}）内，你仍可随时手动开始。', {
          start: config.schedule_start,
          end: config.schedule_end,
        })
      default:
        return ios ? t('点击「开始专注」，开始本轮工作与休息倒计时。') : t('点击「开始专注」，菜单栏会同步显示倒计时。')
    }
  })()

  const ringPhase = (() => {
    switch (state) {
      case 'working': return t('距离休息')
      case 'break': return t('休息倒计时')
      case 'break_done': return t('已满')
      case 'paused': return t('已暂停')
      case 'outside_hours': return t('时段外')
      default: return t('准备就绪')
    }
  })()

  const waterPct = Math.min(100, Math.round((today.water / Math.max(1, config.water_goal)) * 100))
  const maxWater = Math.max(config.water_goal, ...history.map((d) => d.water), 1)
  const maxBreaks = Math.max(4, ...history.map((d) => d.breaks), 1)

  async function toggleSwitch(key: keyof BreakTimerConfig, value: boolean) {
    if (saving) return
    const next: BreakTimerConfig = { ...draft!, [key]: value }
    setDraft(next)
    setCfgError('')
    setSaving(true)
    try {
      await saveConfig(next)
      setDirty(false)
    } catch (e: any) {
      setDraft(snap!.config)
      setCfgError(e?.message || String(e))
    } finally {
      setSaving(false)
    }
  }

  async function submitForm(e: FormEvent) {
    e.preventDefault()
    if (saving || !draft) return
    setSaving(true)
    setCfgError('')
    setSavedNotice(false)
    try {
      await saveConfig(draft)
      setDirty(false)
      setSavedNotice(true)
      window.setTimeout(() => setSavedNotice(false), 2000)
    } catch (e: any) {
      setCfgError(e?.message || String(e))
    } finally {
      setSaving(false)
    }
  }

  function updateDraft<K extends keyof BreakTimerConfig>(key: K, val: BreakTimerConfig[K]) {
    setDraft((prev) => (prev ? { ...prev, [key]: val } : prev))
    setDirty(true)
    setSavedNotice(false)
  }

  const pillClass = state === 'working' || state === 'break_done'
    ? ' working'
    : state === 'break'
      ? ''
      : ' idle'

  return (
    <>
      <div className="page-head">
        <div className="titles">
          <h1>{t('休息')}</h1>
          <div className="sub">{t('定时提醒喝水和起身活动，久坐工作不伤身')}</div>
        </div>
      </div>

      {error && <div className="error" style={{ marginBottom: 16 }}>{error}</div>}

      <div className="stack">
        <section className="card">
          <div className="break-hero">
            <BreakRing
              state={state}
              remainingSec={remainingSec}
              totalSec={total_sec}
              defaultWorkMinutes={config.work_minutes}
              phaseLabel={ringPhase}
              size={148}
            />
            <div className="break-hero-body">
              <span className={'break-status-pill' + pillClass}>
                <Coffee size={13} />
                {statusText}
              </span>
              <div className="ink2">{phaseHint}</div>
              <div className="row">
                {(state === 'idle' || state === 'outside_hours') && (
                  <>
                    <button type="button" className="btn primary" disabled={busy} onClick={() => { void act('start') }}>
                      <Play size={15} />{t('开始专注')}
                    </button>
                    <button type="button" className="btn" disabled={busy} onClick={() => { void act('break_now') }}>
                      <Coffee size={15} />{t('现在休息')}
                    </button>
                  </>
                )}

                {state === 'working' && (
                  <>
                    <button type="button" className="btn" disabled={busy} onClick={() => { void act('pause') }}>
                      <Pause size={15} />{t('暂停计时')}
                    </button>
                    <button type="button" className="btn primary" disabled={busy} onClick={() => { void act('break_now') }}>
                      <Coffee size={15} />{t('现在休息')}
                    </button>
                    <button type="button" className="btn" disabled={busy} onClick={() => { void act('snooze') }}>
                      <Clock size={15} />{t('推迟 {m} 分钟', { m: config.snooze_minutes })}
                    </button>
                    <button type="button" className="btn ghost" disabled={busy} onClick={() => { void act('stop') }}>
                      <RotateCcw size={14} />{t('停止')}
                    </button>
                  </>
                )}

                {state === 'paused' && (
                  <>
                    <button type="button" className="btn primary" disabled={busy} onClick={() => { void act('resume') }}>
                      <Play size={15} />{t('继续计时')}
                    </button>
                    <button type="button" className="btn" disabled={busy} onClick={() => { void act('break_now') }}>
                      <Coffee size={15} />{t('现在休息')}
                    </button>
                    <button type="button" className="btn" disabled={busy} onClick={() => { void act('snooze') }}>
                      <Clock size={15} />{t('推迟 {m} 分钟', { m: config.snooze_minutes })}
                    </button>
                    <button type="button" className="btn ghost" disabled={busy} onClick={() => { void act('stop') }}>
                      <RotateCcw size={14} />{t('停止')}
                    </button>
                  </>
                )}

                {state === 'break' && (
                  <>
                    <button type="button" className="btn primary" disabled={busy} onClick={() => { void act('complete') }}>
                      <Play size={15} />{t('休息完了')}
                    </button>
                    <button type="button" className="btn" disabled={busy} onClick={() => { void act('snooze') }}>
                      <Clock size={15} />{t('推迟 {m} 分钟', { m: config.snooze_minutes })}
                    </button>
                    <button type="button" className="btn ghost" disabled={busy} onClick={() => { void act('skip') }}>
                      <SkipForward size={15} />{t('跳过')}
                    </button>
                  </>
                )}

                {state === 'break_done' && (
                  <>
                    <button type="button" className="btn primary" disabled={busy} onClick={() => { void act('complete') }}>
                      <Play size={15} />{t('开始下一轮')}
                    </button>
                    <button type="button" className="btn ghost" disabled={busy} onClick={() => { void act('complete_idle') }}>
                      {t('暂不开始')}
                    </button>
                  </>
                )}
              </div>
            </div>
          </div>
        </section>

        <section className="card">
          <div className="card-head">
            <h2>{t('今日喝水与休息')}</h2>
            <div className="legend">
              <span><i style={{ background: 'var(--c-breaks)' }} />{t('喝水')}</span>
              <span><i style={{ background: 'var(--accent)' }} />{t('休息')}</span>
            </div>
          </div>

          <div className="break-stats">
            <div className="break-stat-box">
              <div className="break-stat-top">
                <span className="row" style={{ gap: 6, fontWeight: 600 }}>
                  <Droplets size={16} style={{ color: 'var(--c-breaks)' }} />
                  {t('今日喝水')}
                </span>
                <div className="row" style={{ gap: 6 }}>
                  <button
                    type="button"
                    className="btn sm sq"
                    aria-label={t('减少一杯')}
                    disabled={busy || today.water <= 0}
                    onClick={() => { void act('water_sub') }}
                  >
                    −
                  </button>
                  <button
                    type="button"
                    className="btn sm primary"
                    disabled={busy}
                    onClick={() => { void act('water_add') }}
                    style={{ background: 'var(--c-breaks)' } as CSSProperties}
                  >
                    +1
                  </button>
                </div>
              </div>
              <div className="break-stat-num num">
                {today.water}
                <small>{t('/ {n} 杯', { n: config.water_goal })}</small>
              </div>
              <div className="break-progress" aria-hidden>
                <div style={{ width: `${waterPct}%` }} />
              </div>
            </div>

            <div className="break-stat-box">
              <div className="break-stat-top">
                <span className="row" style={{ gap: 6, fontWeight: 600 }}>
                  <Coffee size={16} style={{ color: 'var(--accent)' }} />
                  {t('今日休息')}
                </span>
              </div>
              <div className="break-stat-num num">
                {today.breaks}
                <small>{t('次')}</small>
              </div>
              <div className="small muted">{t('每次建议起身活动并喝水（约 {m} 分钟）', { m: config.break_minutes })}</div>
            </div>
          </div>

          <div className="break-history" aria-label={t('最近 7 天')}>
            {history.map((d, idx) => {
              const isToday = idx === history.length - 1
              const wH = d.water > 0 ? Math.round((d.water / maxWater) * 44) + 4 : 3
              const bH = d.breaks > 0 ? Math.round((d.breaks / maxBreaks) * 44) + 4 : 3
              return (
                <div
                  key={d.date}
                  className={'break-day' + (isToday ? ' today' : '')}
                  title={t('{date}：喝水 {w} 杯，休息 {b} 次', { date: d.date, w: d.water, b: d.breaks })}
                >
                  <div className="break-bars">
                    <div className="break-bar water" style={{ height: wH }} />
                    <div className="break-bar breaks" style={{ height: bH }} />
                  </div>
                  <div className="break-day-lbl num">{isToday ? t('今天') : shortDay(d.date)}</div>
                </div>
              )
            })}
          </div>
        </section>

        <section className="card">
          <div className="card-head">
            <h2>{t('提醒设置')}</h2>
          </div>

          <form onSubmit={submitForm}>
            <div className="form" style={{ marginBottom: 16 }}>
              <label className="field">
                <span className="lbl">{t('工作时长（分钟）')}</span>
                <input
                  className="input num"
                  type="number"
                  min={5}
                  max={180}
                  value={draft.work_minutes}
                  onChange={(e) => updateDraft('work_minutes', Number(e.target.value) || 0)}
                />
                <span className="hint">{t('5–180 分钟，默认 45 分钟')}</span>
              </label>

              <label className="field">
                <span className="lbl">{t('休息时长（分钟）')}</span>
                <input
                  className="input num"
                  type="number"
                  min={1}
                  max={30}
                  value={draft.break_minutes}
                  onChange={(e) => updateDraft('break_minutes', Number(e.target.value) || 0)}
                />
                <span className="hint">{t('1–30 分钟，默认 3 分钟')}</span>
              </label>

              <label className="field">
                <span className="lbl">{t('推迟时长（分钟）')}</span>
                <input
                  className="input num"
                  type="number"
                  min={1}
                  max={30}
                  value={draft.snooze_minutes}
                  onChange={(e) => updateDraft('snooze_minutes', Number(e.target.value) || 0)}
                />
              </label>

              <label className="field">
                <span className="lbl">{t('每日喝水目标（杯）')}</span>
                <input
                  className="input num"
                  type="number"
                  min={1}
                  max={30}
                  value={draft.water_goal}
                  onChange={(e) => updateDraft('water_goal', Number(e.target.value) || 0)}
                />
              </label>
            </div>

            <div>
              {!ios && (
                <>
                  <div className="break-setting-row">
                    <div className="break-setting-info">
                      <div className="break-setting-title">{t('菜单栏显示倒计时')}</div>
                      <div className="break-setting-sub">{t('Mac 菜单栏实时显示剩余时间，Windows 悬停托盘图标可见')}</div>
                    </div>
                    <Switch
                      checked={draft.menu_bar_countdown}
                      onChange={(v) => { void toggleSwitch('menu_bar_countdown', v) }}
                      label={t('菜单栏显示倒计时')}
                    />
                  </div>

                  <div className="break-setting-row">
                    <div className="break-setting-info">
                      <div className="break-setting-title">{t('到点弹出休息浮窗')}</div>
                      <div className="break-setting-sub">{t('桌面端到点自动在屏幕中央弹出轻量休息倒计时卡片')}</div>
                    </div>
                    <Switch
                      checked={draft.popup_enabled}
                      onChange={(v) => { void toggleSwitch('popup_enabled', v) }}
                      label={t('到点弹出休息浮窗')}
                    />
                  </div>
                </>
              )}

              <div className="break-setting-row">
                <div className="break-setting-info">
                  <div className="break-setting-title">{t('到点播放轻柔提示音')}</div>
                  <div className="break-setting-sub">{t('开始休息和休息满时播放柔和提示音')}</div>
                </div>
                <div className="row" style={{ flexWrap: 'nowrap' }}>
                  <button type="button" className="btn sm ghost" onClick={playChime}>
                    <Volume2 size={14} />{t('试听')}
                  </button>
                  <Switch
                    checked={draft.sound_enabled}
                    onChange={(v) => { void toggleSwitch('sound_enabled', v) }}
                    label={t('到点播放轻柔提示音')}
                  />
                </div>
              </div>

              <div className="break-setting-row">
                <div className="break-setting-info">
                  <div className="break-setting-title">{t('按工作时段自动启停')}</div>
                  <div className="break-setting-sub">{t('进入时段自动开始工作计时，下班时间自动停止')}</div>
                </div>
                <Switch
                  checked={draft.auto_schedule}
                  onChange={(v) => { void toggleSwitch('auto_schedule', v) }}
                  label={t('按工作时段自动启停')}
                />
              </div>

              {draft.auto_schedule && (
                <div className="stack" style={{ gap: 12, paddingTop: 10 }}>
                  <div className="form">
                    <label className="field">
                      <span className="lbl">{t('开始时间')}</span>
                      <input
                        className="input num"
                        type="time"
                        value={draft.schedule_start}
                        onChange={(e) => updateDraft('schedule_start', e.target.value)}
                      />
                    </label>
                    <label className="field">
                      <span className="lbl">{t('结束时间')}</span>
                      <input
                        className="input num"
                        type="time"
                        value={draft.schedule_end}
                        onChange={(e) => updateDraft('schedule_end', e.target.value)}
                      />
                    </label>
                  </div>
                  <div className="break-setting-row" style={{ borderTop: 0, paddingTop: 0 }}>
                    <div className="break-setting-info">
                      <div className="break-setting-title">{t('仅工作日（周一至周五）')}</div>
                    </div>
                    <Switch
                      checked={draft.weekdays_only}
                      onChange={(v) => { void toggleSwitch('weekdays_only', v) }}
                      label={t('仅工作日（周一至周五）')}
                    />
                  </div>
                </div>
              )}
            </div>

            {cfgError && <div className="error" style={{ marginTop: 12 }}>{cfgError}</div>}

            <div className="row" style={{ marginTop: 14 }}>
              <button className="btn primary" disabled={saving || !dirty}>
                {saving ? t('保存中…') : t('保存设置')}
              </button>
              {savedNotice && <span className="small" style={{ color: 'var(--accent)' }}>{t('已保存')}</span>}
            </div>
          </form>
        </section>
      </div>
    </>
  )
}
