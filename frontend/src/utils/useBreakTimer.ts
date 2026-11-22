import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type BreakTimerAction, type BreakTimerConfig, type BreakTimerSnapshot } from '../api'
import { playChime } from './chime'

export function formatMMSS(sec: number): string {
  const s = Math.max(0, Math.floor(sec))
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}

export function useBreakTimer() {
  const [snap, setSnap] = useState<BreakTimerSnapshot | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [fetchedAt, setFetchedAt] = useState(() => Date.now())
  const [nowMs, setNowMs] = useState(() => Date.now())
  const zeroSyncedRef = useRef(false)

  const applySnap = useCallback((next: BreakTimerSnapshot) => {
    const t = Date.now()
    zeroSyncedRef.current = false
    setSnap(next)
    setFetchedAt(t)
    setNowMs(t)
    setError('')
  }, [])

  const reload = useCallback(async () => {
    try {
      applySnap(await api.breakTimer())
    } catch (e: any) {
      setError(e?.message || String(e))
    }
  }, [applySnap])

  useEffect(() => {
    void reload()
    const onChange = () => { void reload() }
    const onChime = () => { playChime() }
    const onVis = () => { if (!document.hidden) void reload() }
    window.addEventListener('break-timer-changed', onChange)
    window.addEventListener('break-timer-chime', onChime)
    window.addEventListener('focus', onChange)
    document.addEventListener('visibilitychange', onVis)
    const syncId = window.setInterval(() => { void reload() }, 15_000)
    return () => {
      window.removeEventListener('break-timer-changed', onChange)
      window.removeEventListener('break-timer-chime', onChime)
      window.removeEventListener('focus', onChange)
      document.removeEventListener('visibilitychange', onVis)
      window.clearInterval(syncId)
    }
  }, [reload])

  const ticking = snap?.state === 'working' || snap?.state === 'break'
  useEffect(() => {
    if (!ticking) return
    const id = window.setInterval(() => setNowMs(Date.now()), 500)
    return () => window.clearInterval(id)
  }, [ticking])

  const remainingSec = snap
    ? ticking
      ? Math.max(0, snap.remaining_sec - Math.max(0, Math.floor((nowMs - fetchedAt) / 1000)))
      : snap.remaining_sec
    : 0

  useEffect(() => {
    if (ticking && remainingSec === 0 && !zeroSyncedRef.current) {
      zeroSyncedRef.current = true
      const id = window.setTimeout(() => { void reload() }, 400)
      return () => window.clearTimeout(id)
    }
  }, [ticking, remainingSec, reload])

  const act = useCallback(async (action: BreakTimerAction) => {
    if (busy) return
    setBusy(true)
    try {
      applySnap(await api.breakTimerAction(action))
    } catch (e: any) {
      setError(e?.message || String(e))
    } finally {
      setBusy(false)
    }
  }, [busy, applySnap])

  const saveConfig = useCallback(async (cfg: BreakTimerConfig) => {
    const next = await api.setBreakTimerConfig(cfg)
    applySnap(next)
    return next
  }, [applySnap])

  return { snap, remainingSec, error, busy, reload, act, saveConfig }
}
