// iOS App Store shell detection and native WKWebView bridge.
// Native injects window.__HEALTH_TRAIL_IOS__ at document start and registers
// window.webkit.messageHandlers.healthTrail in both Local and Remote modes.

export interface IOSBridgeInfo {
  version?: number
  mode?: 'local' | 'remote'
  remoteHost?: string
  nativeRemoteBanner?: boolean
  speech?: boolean
}

export interface IOSSpeechEvent {
  type: 'started' | 'partial' | 'final' | 'stopped' | 'error'
  text?: string
  error?: string
}

interface IOSWindow extends Window {
  __HEALTH_TRAIL_IOS__?: IOSBridgeInfo
  __HEALTH_TRAIL_ON_SPEECH__?: (payload: IOSSpeechEvent) => void
  webkit?: {
    messageHandlers?: {
      healthTrail?: {
        postMessage: (msg: unknown) => void
      }
    }
  }
}

let backendMobile = false

export function markIOSBackend(mobile: boolean) {
  backendMobile = mobile
  if (mobile) document.documentElement.classList.add('ios-app')
}

export function iosBridgeInfo(): IOSBridgeInfo | null {
  const w = window as IOSWindow
  return w.__HEALTH_TRAIL_IOS__ ?? null
}

export function isIOSApp(): boolean {
  const w = window as IOSWindow
  return (
    backendMobile ||
    Boolean(w.__HEALTH_TRAIL_IOS__) ||
    Boolean(w.webkit?.messageHandlers?.healthTrail) ||
    document.documentElement.classList.contains('ios-app')
  )
}

export function iosMode(): 'local' | 'remote' {
  return iosBridgeInfo()?.mode === 'remote' ? 'remote' : 'local'
}

export function openConnectionSettings(): boolean {
  const handler = (window as IOSWindow).webkit?.messageHandlers?.healthTrail
  if (!handler || typeof handler.postMessage !== 'function') return false
  handler.postMessage({ action: 'openConnectionSettings' })
  return true
}

export function hasIOSSpeech(): boolean {
  const w = window as IOSWindow
  return Boolean(w.__HEALTH_TRAIL_IOS__?.speech && typeof w.webkit?.messageHandlers?.healthTrail?.postMessage === 'function')
}

export function startIOSSpeech(lang: 'zh' | 'en'): boolean {
  const handler = (window as IOSWindow).webkit?.messageHandlers?.healthTrail
  if (!handler || typeof handler.postMessage !== 'function') return false
  handler.postMessage({ action: 'startSpeech', lang })
  return true
}

export function stopIOSSpeech(): boolean {
  const handler = (window as IOSWindow).webkit?.messageHandlers?.healthTrail
  if (!handler || typeof handler.postMessage !== 'function') return false
  handler.postMessage({ action: 'stopSpeech' })
  return true
}

const speechListeners = new Set<(ev: IOSSpeechEvent) => void>()

export function subscribeIOSSpeech(fn: (ev: IOSSpeechEvent) => void): () => void {
  const w = window as IOSWindow
  speechListeners.add(fn)
  if (!w.__HEALTH_TRAIL_ON_SPEECH__) {
    w.__HEALTH_TRAIL_ON_SPEECH__ = (payload: IOSSpeechEvent) => {
      for (const cb of speechListeners) cb(payload)
    }
  }
  return () => {
    speechListeners.delete(fn)
    if (speechListeners.size === 0) delete w.__HEALTH_TRAIL_ON_SPEECH__
  }
}
