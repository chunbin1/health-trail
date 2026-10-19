import { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { api, type Schema } from './api'
import { App } from './App'
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

function Root() {
  const [schema, setSchema] = useState<Schema>()
  const [error, setError] = useState('')
  useEffect(() => {
    api.schema().then(setSchema).catch((e) => setError(e.message))
  }, [])
  if (error) return <div className="main"><div className="error">无法连接到健康档案服务：{error}</div></div>
  if (!schema) return null
  return (
    <AppProvider schema={schema}>
      <App />
    </AppProvider>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
