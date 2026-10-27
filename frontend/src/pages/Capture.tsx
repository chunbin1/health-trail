import { useEffect, useRef, useState, type ClipboardEvent, type KeyboardEvent } from 'react'
import { Check, ImagePlus, Mic, SendHorizontal, X } from 'lucide-react'
import { api, fmtDate, type AssistantConfig, type ChatHead, type ChatMsgWire, type ChatTurn, type DraftItem, type DraftStatus, type Entity, type Field, type LabDraft } from '../api'
import { DateTimeInput } from '../components/DateTimeInput'
import { AssistantSetup } from '../components/AssistantSetup'
import { t, tx } from '../i18n'
import { openRecord, useApp, useLoad } from '../state'
import { IconButton, KindIcon } from '../ui'

const FLAGS = ['正常', '偏高', '偏低', '异常']
const CAPTURE_KEY = 'health-trail.capture'
const KINDS = ['image/jpeg', 'image/png', 'image/gif', 'image/webp']

interface Pic { mime: string; data: string; url: string }

interface ChatMsg {
  id: string
  role: 'user' | 'assistant'
  text: string
  images?: Pic[]
  imageCount?: number
  items?: DraftItem[] | null
  unclear?: string
  reply?: string
  raw?: string
  usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number; cached_tokens?: number }
  error?: string
  saved?: Record<number, { type?: string; id?: number }>
  locked?: boolean
  open?: Record<number, boolean>
}

interface KeptCapture {
  id: string
  messages: ChatMsg[]
  draft: string
  pendingId: string
}

function loadCapture(): KeptCapture {
  const empty = { id: '', messages: [], draft: '', pendingId: '' }
  try {
    const raw = sessionStorage.getItem(CAPTURE_KEY)
    if (!raw) return empty
    const v = JSON.parse(raw) as KeptCapture
    return { id: v.id || '', messages: v.messages || [], draft: v.draft || '', pendingId: v.pendingId || '' }
  } catch {
    return empty
  }
}

function remember(d: KeptCapture) {
  try {
    const slim: KeptCapture = {
      id: d.id,
      draft: d.draft,
      pendingId: d.pendingId,
      messages: d.messages.map((m) => ({
        ...m,
        images: undefined,
        imageCount: m.images?.length || m.imageCount || 0,
      })),
    }
    if (!slim.messages.length && !slim.draft && !slim.pendingId && !slim.id) sessionStorage.removeItem(CAPTURE_KEY)
    else sessionStorage.setItem(CAPTURE_KEY, JSON.stringify(slim))
  } catch {
    // private mode: the mounted page still keeps it
  }
}

function turnsOf(msgs: ChatMsg[]): ChatTurn[] {
  const out: ChatTurn[] = []
  const users = msgs.filter((m) => m.role === 'user')
  const keepPics = new Set(users.slice(-2).map((m) => m.id))
  for (const m of msgs.slice(-12)) {
    if (m.role === 'user') {
      out.push({
        role: 'user',
        text: m.text,
        images: keepPics.has(m.id) ? m.images?.filter((p) => p.data).map((p) => ({ mime: p.mime, data: p.data })) : undefined,
      })
    } else if (m.reply || m.items || m.unclear) {
      const mark = m.saved && Object.keys(m.saved).length ? '（用户已保存）' : '（用户还没保存）'
      const body = [m.reply, JSON.stringify({ items: m.items || [], unclear: m.unclear || '' })].filter(Boolean).join('\n')
      out.push({ role: 'assistant', text: body + mark })
    }
  }
  return out
}

function fitImage(file: File): Promise<Pic> {
  if (!KINDS.includes(file.type)) return Promise.reject(new Error(t('只能发送图片（JPG、PNG、GIF、WebP）')))
  if (file.size > 12 * 1024 * 1024) return Promise.reject(new Error(t('这张图片太大')))
  return new Promise((resolve, reject) => {
    const img = new Image()
    const url = URL.createObjectURL(file)
    img.onload = () => {
      const max = 1600
      const scale = Math.min(1, max / Math.max(img.width, img.height, 1))
      const w = Math.max(1, Math.round(img.width * scale))
      const h = Math.max(1, Math.round(img.height * scale))
      const c = document.createElement('canvas')
      c.width = w
      c.height = h
      const g = c.getContext('2d')
      if (!g) {
        URL.revokeObjectURL(url)
        reject(new Error(t('这张图片读不出来')))
        return
      }
      g.drawImage(img, 0, 0, w, h)
      URL.revokeObjectURL(url)
      const mime = file.type === 'image/png' ? 'image/png' : 'image/jpeg'
      const dataUrl = c.toDataURL(mime, 0.82)
      resolve({ mime, data: dataUrl.split(',')[1] || '', url: dataUrl })
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error(t('这张图片读不出来')))
    }
    img.src = url
  })
}

function SaveMode({ auto, onChange }: { auto: boolean; onChange: (on: boolean) => void }) {
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const pick = useRef(onChange)
  pick.current = onChange
  useEffect(() => {
    if (!open) return
    const down = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false)
    }
    const key = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'Escape') { e.stopImmediatePropagation(); setOpen(false); return }
      if (e.key !== '1' && e.key !== '2') return
      e.preventDefault()
      pick.current(e.key === '1')
      setOpen(false)
    }
    document.addEventListener('mousedown', down)
    document.addEventListener('keydown', key)
    return () => {
      document.removeEventListener('mousedown', down)
      document.removeEventListener('keydown', key)
    }
  }, [open])
  const modes = [
    { on: true, key: '1', title: t('自动保存'), desc: t('回复一到就写入档案，用药要另外点保存'), recommend: true },
    { on: false, key: '2', title: t('手动保存'), desc: t('看过、改过之后再保存'), recommend: false },
  ]
  return (
    <div className="mode-picker" ref={box}>
      <button type="button" className="mode-trigger" aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen((v) => !v)}>
        {auto ? t('自动保存') : t('手动保存')}
      </button>
      {open && (
        <div className="mode-menu" role="menu">
          <div className="mode-head">{t('模式')}</div>
          {modes.map((m) => (
            <button type="button" role="menuitem" key={m.key} className="mode-item" onClick={() => { onChange(m.on); setOpen(false) }}>
              <span className="mode-copy">
                <span className="mode-title">
                  {m.title}
                  {m.recommend && <span className="mode-badge">{t('推荐')}</span>}
                </span>
                <span className="mode-desc">{m.desc}</span>
              </span>
              <span className="mode-side">
                {auto === m.on && <Check size={16} />}
                <span className="mode-key">{m.key}</span>
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export function Capture({ panel, onClose }: { panel?: boolean; onClose?: () => void } = {}) {
  const { entity, changed, toast } = useApp()
  const { data, error, reload } = useLoad(() => api.assistant(), [])
  const kept = useRef(loadCapture()).current
  const [cfg, setCfg] = useState<AssistantConfig | null>(null)
  const [showSetup, setShowSetup] = useState(false)
  const [chatId, setChatId] = useState(kept.id || String(Date.now()))
  const [past, setPast] = useState<ChatHead[] | null>(null)
  const [chats, setChats] = useState<ChatHead[]>([])
  const [messages, setMessages] = useState<ChatMsg[]>(kept.messages)
  const [text, setText] = useState(kept.draft)
  const [pics, setPics] = useState<Pic[]>([])
  const [pendingId, setPendingId] = useState(kept.pendingId)
  const [listening, setListening] = useState(false)
  const [micBusy, setMicBusy] = useState(false)
  const [savingId, setSavingId] = useState('')
  const [err, setErr] = useState('')
  const textRef = useRef(kept.draft)
  const pendingRef = useRef(kept.pendingId)
  const applied = useRef('')
  const autoTried = useRef(new Set<string>())
  const fileRef = useRef<HTMLInputElement>(null)
  const savingRef = useRef('')
  const endRef = useRef<HTMLDivElement>(null)
  const view = cfg || data
  const auto = !!view && view.auto_save !== false
  const waiting = !!pendingId

  useEffect(() => {
    remember({ id: chatId, messages, draft: text, pendingId })
  }, [chatId, messages, text, pendingId])

  // Pictures are left out of the browser's short-term copy. If this chat had
  // any, load the copy kept on this computer so they show again.
  useEffect(() => {
    const missing = kept.messages.some((m) => (m.imageCount || 0) > 0 && !m.images?.some((p) => p.url))
    if (!kept.id || !missing) return
    api.assistantChat(kept.id).then((c) => setMessages((c.messages || []).map(restorePics)), () => {})
    // kept is the copy from the first render
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (panel) return
    api.assistantChats().then(setChats, () => {})
  }, [panel])

  useEffect(() => {
    if (!messages.length) return
    api.saveAssistantChat({ id: chatId, messages: wireOf(messages) }).then((c) => {
      setChats((list) => {
        const i = list.findIndex((x) => x.id === c.id)
        if (i < 0) return [c, ...list]
        const next = list.slice()
        next[i] = c
        return next
      })
    }, () => {})
  }, [chatId, messages])

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'end' })
  }, [messages, pendingId])

  // Automatic saving is on unless switched off. A reply that failed to save
  // stays on screen with the button, and is not tried again by itself.
  useEffect(() => {
    if (!auto) return
    const last = messages[messages.length - 1]
    if (!last || last.role !== 'assistant' || autoTried.current.has(last.id)) return
    if (!last.items?.length || savedAll(last) || last.locked) return
    const rest = last.items.filter((it, i) => !last.saved?.[i] && !isMedication(it))
    if (!rest.length || rest.some((it) => it.problem)) return
    autoTried.current.add(last.id)
    save(last, true)
    // save is recreated each render and reads the latest messages for pictures
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages, auto])

  useEffect(() => {
    if (!pendingId) return
    const since = Date.now()
    let stop = false
    const look = async () => {
      let s: DraftStatus | null = null
      try {
        s = await api.assistantDraftStatus()
      } catch {
        return
      }
      if (stop || s.id !== pendingRef.current) return
      if (s.state === 'done') {
        takeReply(s.id, s)
        return
      }
      if (s.state === 'error') {
        setErr(s.error || t('整理被中断了，刚写的还在，再点一次「发送」。'))
        setPendingId('')
        pendingRef.current = ''
        return
      }
      if (s.state === 'none' && Date.now() - since > 60_000) {
        setErr(t('整理被中断了，刚写的还在，再点一次「发送」。'))
        setPendingId('')
        pendingRef.current = ''
      }
    }
    look()
    const id = setInterval(look, 600)
    return () => {
      stop = true
      clearInterval(id)
    }
  }, [pendingId])

  useEffect(() => {
    if (!listening) return
    const id = setInterval(() => {
      api.speech().then((s) => {
        if (s.text) writeText(s.text)
        if (s.error) setErr(s.error)
        if (!s.listening) setListening(false)
      }, () => {})
    }, 400)
    return () => clearInterval(id)
  }, [listening])

  if (error) return <div className="error">{error}</div>
  if (!view) return null

  function writeText(v: string) {
    textRef.current = v
    setText(v)
  }

  function takeReply(id: string, d: { items?: DraftItem[]; unclear?: string; reply?: string; raw?: string; usage?: ChatMsg['usage'] }) {
    if (!id || applied.current === id) return
    applied.current = id
    setMessages((ms) => [...ms, {
      id: id + '-a', role: 'assistant', text: '', items: d.items || [], unclear: d.unclear || '',
      reply: d.reply || '', usage: d.usage,
    }])
    setPendingId('')
    pendingRef.current = ''
    setErr('')
  }

  async function addFiles(files: File[]) {
    setErr('')
    const room = 4 - pics.length
    if (room <= 0) {
      setErr(t('一次最多 4 张图片'))
      return
    }
    const next: Pic[] = []
    for (const f of files.slice(0, room)) {
      try {
        next.push(await fitImage(f))
      } catch (e: any) {
        setErr(e.message)
      }
    }
    if (files.length > room) setErr(t('一次最多 4 张图片'))
    setPics((cur) => [...cur, ...next].slice(0, 4))
  }

  function onPaste(e: ClipboardEvent<HTMLTextAreaElement>) {
    const files = [...e.clipboardData.files]
    if (!files.length) return
    e.preventDefault()
    addFiles(files)
  }

  async function mic() {
    if (micBusy || waiting) return
    setMicBusy(true)
    setErr('')
    try {
      if (!listening) {
        const s = await api.speechStart()
        setListening(s.listening)
        if (s.error) setErr(s.error)
        if (s.text) writeText(s.text)
      } else {
        const s = await api.speechStop()
        setListening(false)
        if (s.text) writeText(s.text)
      }
    } catch (e: any) {
      setListening(false)
      setErr(e.message)
    } finally {
      setMicBusy(false)
    }
  }

  async function send() {
    if (waiting || micBusy) return
    setErr('')
    let said = textRef.current
    if (listening) {
      const s = await api.speechStop()
      setListening(false)
      if (s.text) said = s.text
    }
    said = said.trim()
    if (!said && !pics.length) {
      setErr(t('先写一点内容，或点麦克风说。'))
      return
    }
    const id = String(Date.now())
    const user: ChatMsg = { id, role: 'user', text: said, images: pics }
    const history = [...messages, user]
    setMessages(history)
    writeText('')
    setPics([])
    setPendingId(id)
    pendingRef.current = id
    remember({ id: chatId, messages: history, draft: '', pendingId: id })
    try {
      const d = await api.assistantDraft({ id, messages: turnsOf(history) })
      takeReply(id, d)
    } catch {
      setPendingId(id)
      pendingRef.current = id
    }
  }

  function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  function picsBefore(msg: ChatMsg): Pic[] {
    const i = messages.findIndex((m) => m.id === msg.id)
    for (let j = i - 1; j >= 0; j--) {
      if (messages[j].role === 'user') return (messages[j].images || []).filter((p) => p.data)
    }
    return []
  }

  async function save(msg: ChatMsg, autoSave = false) {
    if (savingRef.current || savingId || msg.locked || savedAll(msg) || !msg.items?.length) return
    const pending = msg.items.map((it, i) => ({ it, i })).filter(({ it, i }) => !msg.saved?.[i] && !(autoSave && isMedication(it)))
    if (!pending.length) return
    if (pending.some(({ it }) => it.problem)) {
      setErr(t('请先改掉标出的问题，再保存。'))
      return
    }
    const prior = messages.slice(0, messages.findIndex((m) => m.id === msg.id)).reverse().find((m) => m.role === 'user')
    if ((prior?.imageCount || prior?.images?.length) && !prior.images?.some((p) => p.data)) {
      setErr(t('这张图片已经不在这次对话里，请重新发一次再保存。'))
      return
    }
    savingRef.current = msg.id
    setSavingId(msg.id)
    setErr('')
    try {
      const res = await api.assistantCommit(pending.map(({ it }) => cleanItem(it)))
      const done: Record<number, { type?: string; id?: number }> = { ...msg.saved }
      for (const s of res.saved || []) {
        const at = pending[s.index]
        if (at) done[at.i] = { type: s.type, id: s.id }
      }
      const wrote = !!res.saved?.length
      const unfinished = msg.items.some((_, i) => !done[i])
      setMessages((ms) => ms.map((m) => m.id === msg.id ? { ...m, saved: done, locked: (!res.ok && wrote) || (res.ok && !unfinished) } : m))
      if (!res.ok) {
        setErr(res.error || '')
        if (wrote) changed()
        return
      }
      const pics = picsBefore(msg)
      const reports = (res.saved || []).filter((s) => s.type === 'reports' && s.id)
      const targets = reports.length ? reports : (res.saved || []).filter((s) => s.id).slice(0, 1)
      try {
        for (const s of targets) {
          for (let n = 0; n < pics.length; n++) {
            const p = pics[n]
            const ext = p.mime === 'image/png' ? 'png' : p.mime === 'image/gif' ? 'gif' : p.mime === 'image/webp' ? 'webp' : 'jpg'
            await api.uploadBase64(s.type!, s.id!, `图片${n + 1}.${ext}`, p.mime, p.data)
          }
        }
      } catch (e: any) {
        changed()
        setErr(t('记录已保存，但图片没能存成附件：{error}', { error: e.message }))
        return
      }
      changed()
      toast(t('已记入档案'))
    } catch (e: any) {
      setErr(e.message)
    } finally {
      savingRef.current = ''
      setSavingId('')
    }
  }

  function patch(msgId: string, i: number, next: DraftItem) {
    setMessages((ms) => ms.map((m) => m.id === msgId ? { ...m, items: m.items?.map((it, n) => n === i ? { ...next, problem: '' } : it) } : m))
  }

  function newChat() {
    if (messages.length) api.saveAssistantChat({ id: chatId, messages: wireOf(messages) }).catch(() => {})
    setChatId(String(Date.now()))
    setPast(null)
    setMessages([])
    setPendingId('')
    pendingRef.current = ''
    writeText('')
    setPics([])
    setErr('')
  }

  async function openPast() {
    setErr('')
    try {
      setPast(await api.assistantChats())
    } catch (e: any) {
      setErr(e.message)
    }
  }

  async function openPastChat(id: string) {
    setErr('')
    try {
      const c = await api.assistantChat(id)
      setChatId(c.id)
      setMessages((c.messages || []).map(restorePics))
      setPendingId('')
      pendingRef.current = ''
      setPast(null)
    } catch (e: any) {
      setErr(e.message)
    }
  }

  const micLabel = listening ? t('结束听写') : micBusy ? t('正在听…') : t('开始听')

  return (
    <div className={panel ? 'capture-panel' : 'capture-page'}>
      {!panel && (
        <aside className="chat-side">
          <button type="button" className="btn sm" onClick={newChat}>{t('新对话')}</button>
          <div className="chat-side-list">
            {chats.length === 0 && <div className="sub">{t('还没有对话记录')}</div>}
            {chats.map((c) => (
              <button type="button" key={c.id} className={'chat-side-item' + (c.id === chatId ? ' on' : '')} onClick={() => { if (c.id !== chatId) openPastChat(c.id) }}>
                <span className="title">{c.title}</span>
                <span className="small muted">{chatMeta(c)}</span>
              </button>
            ))}
          </div>
        </aside>
      )}
      <div className="chat-main">
      <div className="page-head">
        <div className="titles">
          <h1>{t('AI助手')}</h1>
          <div className="sub">{t('像聊天一样说，也可以问档案里的报告和症状，或发化验单、处方的照片。要记下的内容，自动保存开着时会写入档案。用药建议不会自动写入，要点保存。')}</div>
        </div>
        {view.configured && !showSetup && (
          <button type="button" className="btn ghost sm" onClick={() => setShowSetup(true)}>{t('修改接口')}</button>
        )}
        {!panel && (
          <button type="button" className="btn ghost sm phone-only" onClick={() => { past ? setPast(null) : openPast() }}>{past ? t('返回') : t('历史')}</button>
        )}
        {panel && messages.length > 0 && !past && (
          <button type="button" className="btn sm" onClick={newChat}>
            {t('新对话')}
          </button>
        )}
        {panel && onClose && (
          <IconButton label={t('关闭')} onClick={onClose}><X size={16} /></IconButton>
        )}
      </div>
      <div className="chat-top">
        <div className="notice">
          <ImagePlus size={18} />
          <span>{t('声音只在这台 Mac 上识别，不会上传。文字和图片会发给你填写的接口，同时带上已有健康问题、用药和打卡项的名称。整理出来的内容不能代替医生的诊断。')}</span>
        </div>
        {(!view.configured || showSetup) && (
          <section className="card">
            {!view.configured && <p className="sub" style={{ marginTop: 0 }}>{t('还没有填写接口。填好以后就可以在这里说话或打字记档案。')}</p>}
            <AssistantSetup cfg={view} onSaved={(c) => { setCfg(c); setShowSetup(false); reload() }} />
          </section>
        )}
        {err && <div className="error">{err}</div>}
      </div>
      <div className="stack">
        <div className="chat-log">
          {past ? (
            past.length === 0
              ? <div className="sub">{t('还没有对话记录')}</div>
              : <div className="card flush">{past.map((c) => (
                <button type="button" key={c.id} className="row-item no-ico" onClick={() => openPastChat(c.id)}>
                  <span className="body">{c.title}</span>
                  <span className="small muted nowrap">{chatMeta(c)}</span>
                </button>
              ))}</div>
          ) : messages.map((m) => m.role === 'user' ? (
            <div key={m.id} className="bubble-user">
              {(m.images?.length || m.imageCount) ? (
                <div className="chat-pics" style={{ marginBottom: m.text ? 8 : 0 }}>
                  {m.images?.filter((p) => p.url).map((p, i) => <img key={i} src={p.url} alt="" />)}
                  {!m.images?.some((p) => p.url) && m.imageCount ? <span className="small muted">{t('（图片）')}</span> : null}
                </div>
              ) : null}
              {m.text}
            </div>
          ) : (
            <div key={m.id} className="chat-turn">
              {m.reply && <div className="bubble-assistant">{m.reply}</div>}
              {m.unclear && <div className="notice"><span>{m.unclear}</span></div>}
              {m.items && m.items.length === 0 && !m.unclear && !m.reply && <div className="sub">{t('没有整理出可以保存的记录。')}</div>}
              {m.items?.map((it, i) => (
                <section key={i} className="card">
                  <div className="card-head">
                    <KindIcon kind={iconKind(it)} size="sm" />
                    <h2 style={{ flex: 1, minWidth: 0 }}>{itemTitle(it, entity)}</h2>
                    {itemDate(it, entity) && <span className="small muted nowrap">{itemDate(it, entity)}</span>}
                    {m.saved?.[i] && <span className="tag">{t('已保存')}</span>}
                    {!m.locked && !savedAll(m) && (
                      <IconButton label={t('去掉')} small onClick={() => setMessages((ms) => ms.map((x) => x.id === m.id ? { ...x, items: x.items?.filter((_, n) => n !== i) } : x))}>
                        <X size={14} />
                      </IconButton>
                    )}
                  </div>
                  {it.problem && <div className="error" style={{ marginBottom: 8 }}>{it.problem}</div>}
                  {m.saved?.[i]?.id && m.saved[i].type && (
                    <button type="button" className="btn ghost sm" onClick={() => openRecord(m.saved![i].type!, m.saved![i].id!)}>{t('查看')}</button>
                  )}
                  {!m.locked && !savedAll(m) && (
                    <button type="button" className="btn ghost sm" onClick={() => setMessages((ms) => ms.map((x) => x.id === m.id ? { ...x, open: { ...x.open, [i]: !x.open?.[i] } } : x))}>
                      {m.open?.[i] ? t('收起') : t('修改')}
                    </button>
                  )}
                  {(m.open?.[i] || it.problem) && !m.locked && (
                    <ItemEditor item={it} onChange={(next) => patch(m.id, i, next)} />
                  )}
                </section>
              ))}
              {!!m.items?.length && !savedAll(m) && (
                <div className="row">
                  <button type="button" className="btn primary sm" disabled={!!savingId || !!m.locked} onClick={() => save(m)}>
                    {savingId === m.id ? t('正在保存…') : t('保存到档案')}
                  </button>
                </div>
              )}
              {m.locked && !savedAll(m) && <div className="notice warn"><span>{t('已经写进档案的已标出。为避免记重，这次不能再保存。还没写上的，请到对应的页面补记。')}</span></div>}
              {!!m.usage?.total_tokens && (
                <div className="small muted">{t('输入 {prompt} · 输出 {completion} · 合计 {total} · 缓存 {cached}', {
                  prompt: m.usage.prompt_tokens, completion: m.usage.completion_tokens, total: m.usage.total_tokens,
                  cached: m.usage.cached_tokens || 0,
                })}</div>
              )}
            </div>
          ))}
          {waiting && <div className="sub">{t('正在思考…')}</div>}
          <div ref={endRef} />
        </div>

        {view.configured && (
          <div className="chat-bar">
            {pics.length > 0 && (
              <div className="chat-pics" style={{ marginBottom: 8 }}>
                {pics.map((p, i) => (
                  <div key={i} className="chat-thumb">
                    <img src={p.url} alt="" />
                    <IconButton label={t('去掉')} small onClick={() => setPics(pics.filter((_, n) => n !== i))}><X size={12} /></IconButton>
                  </div>
                ))}
              </div>
            )}
            <textarea className="input" style={{ width: '100%' }} rows={3} value={text}
              placeholder={t('输入消息，或发一张化验单。Enter 发送')}
              onChange={(e) => writeText(e.target.value)} onKeyDown={onKey} onPaste={onPaste} />
            <div className="row" style={{ marginTop: 8 }}>
              <SaveMode auto={auto} onChange={(on) => {
                api.setAssistant({ auto_save: on }).then((c) => setCfg(c), (e) => setErr(e.message))
              }} />
              <input ref={fileRef} type="file" accept={KINDS.join(',')} multiple hidden onChange={(e) => { addFiles([...e.target.files || []]); e.target.value = '' }} />
              <button type="button" className="btn sm" disabled={waiting} onClick={() => fileRef.current?.click()}>
                <ImagePlus size={14} />{t('添加图片')}
              </button>
              {view.speech && (
                <button type="button" className={'btn sm' + (listening ? ' primary' : '')} disabled={(micBusy && !listening) || waiting} onClick={mic}>
                  <Mic size={14} />{micLabel}
                </button>
              )}
              <button type="button" className="btn primary sm" disabled={waiting || micBusy} onClick={send}>
                <SendHorizontal size={14} />{waiting ? t('正在思考…') : t('发送')}
              </button>
            </div>
          </div>
        )}
      </div>
      </div>
    </div>
  )
}

function chatMeta(c: ChatHead) {
  return c.tokens ? c.updated + ' · ' + t('{n} token', { n: c.tokens }) : c.updated
}

function restorePics(m: ChatMsgWire): ChatMsg {
  const images = m.images?.filter((p) => p.data).map((p) => ({
    mime: p.mime, data: p.data, url: `data:${p.mime};base64,${p.data}`,
  }))
  return { ...m, images, imageCount: images?.length || m.imageCount }
}

function wireOf(messages: ChatMsg[]): ChatMsgWire[] {
  return messages.map((m) => ({
    id: m.id, role: m.role, text: m.text,
    images: m.images?.filter((p) => p.data).map((p) => ({ mime: p.mime, data: p.data })),
    imageCount: m.images?.length || m.imageCount || 0,
    items: m.items, unclear: m.unclear, reply: m.reply, usage: m.usage, saved: m.saved, locked: m.locked,
  }))
}

function isMedication(it: DraftItem) {
  return it.type === 'medications' || it.kind === 'medications'
}

function savedAll(m: ChatMsg) {
  return !!m.items?.length && !!m.saved && m.items.every((_, i) => m.saved![i])
}

function iconKind(it: DraftItem) {
  if (it.kind === 'checkin') return 'checkins'
  if (it.kind === 'lab_report') return 'lab_results'
  return it.type || 'search'
}

function itemDate(it: DraftItem, entity: (name: string) => Entity | undefined) {
  const e = it.type ? entity(it.type) : undefined
  const raw = e?.dateKey && it.fields ? it.fields[e.dateKey] : ''
  return raw ? fmtDate(String(raw).slice(0, 10)) : ''
}

function itemTitle(it: DraftItem, entity: (name: string) => { label: string; titleKey: string } | undefined) {
  if (it.kind === 'checkin') return it.habit || t('打卡')
  if (it.kind === 'lab_report') return String(it.report?.title || t('化验单'))
  const e = it.type ? entity(it.type) : undefined
  const title = e && it.fields ? it.fields[e.titleKey] : ''
  return String(title || it.summary || (e ? tx(e.label) : it.type || ''))
}

function cleanItem(it: DraftItem): DraftItem {
  const fields: Record<string, any> = {}
  for (const [k, v] of Object.entries(it.fields || {})) {
    if (v === '' || v == null) continue
    fields[k] = v
  }
  const { problem: _p, ...rest } = it
  return { ...rest, fields }
}

function ItemEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  if (item.kind === 'checkin') return <CheckinEditor item={item} onChange={onChange} />
  if (item.kind === 'lab_report') return <LabEditor item={item} onChange={onChange} />
  return <RecordEditor item={item} onChange={onChange} />
}

function RecordEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  const { entity } = useApp()
  const e = item.type ? entity(item.type) : undefined
  const [all, setAll] = useState(false)
  if (!e) return null
  const fields = item.fields || {}
  const shown = e.fields.filter((f) => all || f.required || (fields[f.name] != null && fields[f.name] !== ''))
  const set = (k: string, v: any) => onChange({ ...item, fields: { ...fields, [k]: v } })
  const hasCond = e.fields.some((f) => f.name === 'condition_id')
  const hasMed = e.fields.some((f) => f.name === 'medication_id')
  return (
    <div className="form" style={{ marginTop: 12 }}>
      {shown.map((f) => (
        <FieldInput key={f.name} field={f} value={fields[f.name]} onChange={(v) => set(f.name, v)} wide={f.type === 'longtext'} />
      ))}
      {hasCond && (
        <label className="field full">
          <span className="lbl">{t('相关健康问题的名称')}</span>
          <input className="input" value={item.condition_name || ''} onChange={(e) => onChange({ ...item, condition_name: e.target.value })} />
        </label>
      )}
      {hasMed && (
        <label className="field full">
          <span className="lbl">{t('对应药物的名称')}</span>
          <input className="input" value={item.medication_name || ''} onChange={(e) => onChange({ ...item, medication_name: e.target.value })} />
        </label>
      )}
      <div className="full">
        <button type="button" className="btn ghost sm" onClick={() => setAll(!all)}>{all ? t('收起') : t('全部字段')}</button>
      </div>
    </div>
  )
}

function FieldInput({ field, value, onChange, wide }: { field: Field; value: any; onChange: (v: any) => void; wide?: boolean }) {
  const v = value ?? ''
  if (field.type === 'bool') return null
  const box = field.type === 'date' || field.type === 'datetime' || field.type === 'time' ? (
    <DateTimeInput mode={field.type} value={String(v)} onChange={onChange} />
  ) : field.type === 'enum' ? (
    <select className="input" value={String(v)} onChange={(e) => onChange(e.target.value)}>
      {!field.required && <option value="">{t('未选择')}</option>}
      {(field.options || []).map((o) => <option key={o} value={o}>{tx(o)}</option>)}
    </select>
  ) : field.type === 'longtext' ? (
    <textarea className="input" value={String(v)} onChange={(e) => onChange(e.target.value)} />
  ) : field.type === 'int' || field.type === 'real' || field.type === 'ref' ? (
    <input className="input" inputMode="decimal" value={v === '' ? '' : String(v)}
      onChange={(e) => onChange(e.target.value === '' ? '' : Number(e.target.value))} />
  ) : (
    <input className="input" value={String(v)} onChange={(e) => onChange(e.target.value)} />
  )
  return (
    <label className={'field' + (wide ? ' full' : '')}>
      <span className="lbl">{tx(field.label)}{field.required && <span className="req">*</span>}</span>
      {box}
    </label>
  )
}

function CheckinEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  return (
    <div className="form" style={{ marginTop: 12 }}>
      <label className="field">
        <span className="lbl">{t('打卡项')}</span>
        <input className="input" value={item.habit || ''} onChange={(e) => onChange({ ...item, habit: e.target.value })} />
      </label>
      <label className="field">
        <span className="lbl">{t('数量')}</span>
        <input className="input" inputMode="decimal" value={item.value ?? ''} onChange={(e) => onChange({ ...item, value: e.target.value === '' ? null : Number(e.target.value) })} />
      </label>
      <label className="field full">
        <span className="lbl">{t('备注')}</span>
        <input className="input" value={item.notes || ''} onChange={(e) => onChange({ ...item, notes: e.target.value })} />
      </label>
    </div>
  )
}

function LabEditor({ item, onChange }: { item: DraftItem; onChange: (it: DraftItem) => void }) {
  const report = item.report || {}
  const setReport = (k: string, v: any) => onChange({ ...item, report: { ...report, [k]: v } })
  const results = item.results || []
  const setRow = (i: number, row: LabDraft) => onChange({ ...item, results: results.map((r, n) => (n === i ? row : r)) })
  return (
    <div className="form" style={{ marginTop: 12 }}>
      <label className="field">
        <span className="lbl">{t('标题')}</span>
        <input className="input" value={report.title || ''} onChange={(e) => setReport('title', e.target.value)} />
      </label>
      <label className="field">
        <span className="lbl">{tx('报告日期')}</span>
        <DateTimeInput mode="date" value={String(report.report_date || '')} onChange={(v) => setReport('report_date', v)} />
      </label>
      <label className="field">
        <span className="lbl">{tx('类型')}</span>
        <select className="input" value={report.report_type || ''} onChange={(e) => setReport('report_type', e.target.value)}>
          <option value="">{t('未选择')}</option>
          {['体检', '化验', '影像', '病理', '其他'].map((o) => <option key={o} value={o}>{tx(o)}</option>)}
        </select>
      </label>
      {results.map((r, i) => (
        <div key={i} className="full form" style={{ padding: 10, border: '1px solid var(--line)', borderRadius: 10 }}>
          <label className="field">
            <span className="lbl">{t('项目')}</span>
            <input className="input" value={r.item_name || ''} onChange={(e) => setRow(i, { ...r, item_name: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{t('结果')}</span>
            <input className="input" value={r.value ?? ''} onChange={(e) => setRow(i, { ...r, value: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{t('单位')}</span>
            <input className="input" value={r.unit || ''} onChange={(e) => setRow(i, { ...r, unit: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{t('参考范围')}</span>
            <input className="input" value={r.ref_range || ''} onChange={(e) => setRow(i, { ...r, ref_range: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl">{tx('标记')}</span>
            <select className="input" value={r.flag || ''} onChange={(e) => setRow(i, { ...r, flag: e.target.value })}>
              <option value="">{t('未选择')}</option>
              {FLAGS.map((f) => <option key={f} value={f}>{tx(f)}</option>)}
            </select>
          </label>
          <div className="field">
            <span className="lbl">&nbsp;</span>
            <button type="button" className="btn sm" onClick={() => onChange({ ...item, results: results.filter((_, n) => n !== i) })}>{t('删掉这条指标')}</button>
          </div>
        </div>
      ))}
      <div className="full">
        <button type="button" className="btn sm" onClick={() => onChange({ ...item, results: [...results, {}] })}>{t('添加一条指标')}</button>
      </div>
    </div>
  )
}
