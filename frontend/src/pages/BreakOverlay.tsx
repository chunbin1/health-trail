import { useEffect, useState } from 'react'
import { Check, Coffee, Droplets, Play, SkipForward, Clock, X } from 'lucide-react'
import { api } from '../api'
import { BreakRing } from '../components/BreakRing'
import { setLang, storedLang, t, type Lang } from '../i18n'
import { useBreakTimer } from '../utils/useBreakTimer'

export function BreakOverlay() {
  const { snap, remainingSec, busy, act } = useBreakTimer()
  const [, setLangTick] = useState(0)

  useEffect(() => {
    setLang(storedLang())
    api.info()
      .then((info) => {
        const l: Lang = info.lang === 'en' ? 'en' : 'zh'
        setLang(l)
        setLangTick((n) => n + 1)
      })
      .catch(() => {})
    const onLang = (e: Event) => {
      const detail = (e as CustomEvent<string>).detail
      setLang(detail === 'en' ? 'en' : 'zh')
      setLangTick((n) => n + 1)
    }
    window.addEventListener('health-trail-lang', onLang)
    return () => window.removeEventListener('health-trail-lang', onLang)
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        void api.hideBreakWindow().catch(() => {})
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  if (!snap) return <div className="break-overlay" />

  const { state, total_sec, today, config } = snap
  const isDone = state === 'break_done'
  const isBreak = state === 'break'

  const badgeLabel = isDone
    ? t('休息结束')
    : isBreak
      ? t('休息时间')
      : state === 'working'
        ? t('专注中')
        : state === 'paused'
          ? t('已暂停')
          : t('休息提醒')

  const ringLabel = isDone
    ? t('已满')
    : isBreak
      ? t('休息倒计时')
      : state === 'working'
        ? t('距离休息')
        : state === 'paused'
          ? t('已暂停')
          : t('未开启')

  const promptText = isDone
    ? t('休息好了吗？坐回电脑前再开始下一轮')
    : t('起来站一会儿，喝口水、远眺放松一下')

  return (
    <div className="break-overlay">
      <div className="break-overlay-head">
        <span className={'break-status-pill' + (isDone ? ' working' : isBreak ? '' : ' idle')}>
          <Coffee size={13} />
          {badgeLabel}
        </span>
        <button
          type="button"
          className="icon-btn sm"
          aria-label={t('关闭')}
          title={t('关闭')}
          onClick={() => { void api.hideBreakWindow().catch(() => {}) }}
        >
          <X size={16} />
        </button>
      </div>

      <div className="break-overlay-center">
        <BreakRing
          state={state}
          remainingSec={remainingSec}
          totalSec={total_sec}
          defaultWorkMinutes={config.work_minutes}
          phaseLabel={ringLabel}
          size={122}
        />
        <div className="break-overlay-title">{promptText}</div>
      </div>

      <div className="break-overlay-actions">
        <div className="break-overlay-row">
          <button
            type="button"
            className="btn"
            disabled={busy}
            onClick={() => { void act('water_add') }}
          >
            <Droplets size={15} style={{ color: 'var(--c-breaks)' }} />
            {t('喝了一杯 (+1)')}
            <span className="tag k num" style={{ '--k': 'var(--c-breaks)' } as React.CSSProperties}>
              {today.water}/{config.water_goal}
            </span>
          </button>

          {isBreak && (
            <button type="button" className="btn primary" disabled={busy} onClick={() => { void act('complete') }}>
              <Check size={15} />
              {t('休息完了')}
            </button>
          )}
          {isDone && (
            <button type="button" className="btn primary" disabled={busy} onClick={() => { void act('complete') }}>
              <Play size={15} />
              {t('开始下一轮')}
            </button>
          )}
          {!isBreak && !isDone && (
            <button
              type="button"
              className="btn primary"
              disabled={busy}
              onClick={() => { void act(state === 'working' || state === 'paused' ? 'break_now' : 'start') }}
            >
              <Play size={15} />
              {state === 'working' || state === 'paused' ? t('现在休息') : t('开始专注')}
            </button>
          )}
        </div>

        <div className="break-overlay-row">
          {isBreak && (
            <>
              <button type="button" className="btn sm ghost" disabled={busy} onClick={() => { void act('snooze') }}>
                <Clock size={13} />
                {t('推迟 {m} 分钟', { m: config.snooze_minutes })}
              </button>
              <button type="button" className="btn sm ghost" disabled={busy} onClick={() => { void act('skip') }}>
                <SkipForward size={13} />
                {t('跳过')}
              </button>
            </>
          )}
          {isDone && (
            <button type="button" className="btn sm ghost" disabled={busy} onClick={() => { void act('complete_idle') }}>
              {t('暂不开始')}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
