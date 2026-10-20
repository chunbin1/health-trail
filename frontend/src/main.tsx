import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { api, type Schema } from './api'
import { App } from './App'
import { setLang, storedLang, t, type Lang } from './i18n'
import { AppProvider } from './state'
import './styles.css'

// In the Mac window (opened at /?shell=mac) the page draws under a hidden
// title bar: the sidebar is translucent and is where the window is dragged.
// Wails' runtime, which makes --wails-draggable work, is served only there.
if (new URLSearchParams(location.search).get('shell') === 'mac') {
  document.documentElement.classList.add('mac-window')
  const s = document.createElement('script')
  s.type = 'module'
  s.src = '/wails/runtime.js'
  document.head.appendChild(s)
}

// The language is kept in the data folder's settings (so the window, the
// browser and the phone agree) and in this browser, for when the server's
// can't be read.
function Root() {
  const [schema, setSchema] = useState<Schema>()
  const [lang, setLangState] = useState<Lang>(storedLang)
  const [error, setError] = useState('')
  useEffect(() => {
    setLang(storedLang())
    Promise.all([api.schema(), api.info().catch(() => null)])
      .then(([s, info]) => {
        const l: Lang = info ? (info.lang === 'en' ? 'en' : 'zh') : storedLang()
        setLang(l, s)
        setLangState(l)
        setSchema(s)
      })
      .catch((e) => setError(e.message))
  }, [])
  if (error) return <div className="main"><div className="error">{t('无法连接到健康档案服务：{error}', { error })}</div></div>
  if (!schema) return null
  function switchLang(l: Lang) {
    setLang(l, schema)
    setLangState(l)
    api.setLang(l).catch(() => {})
  }
  // remounted on a switch, so everything is drawn again in the new language
  return (
    <AppProvider key={lang} schema={schema} lang={lang} switchLang={switchLang}>
      <App />
    </AppProvider>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
