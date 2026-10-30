import { CircleCheck } from 'lucide-react'
import { useState, type MouseEvent } from 'react'
import { api, type AIProvider, type AssistantConfig } from '../api'
import { isEN, t } from '../i18n'
import { useApp } from '../state'
import { AssistantLog } from './AssistantLog'

const hostOf = (u: string) => {
  try { return new URL(u).hostname.toLowerCase() } catch { return '' }
}

// The listed service whose address is saved; a fresh setup starts on the first.
function startOn(cfg: AssistantConfig): string {
  const list = cfg.providers || []
  if (!cfg.base_url) return list[0]?.id ?? 'custom'
  return list.find((p) => hostOf(p.base_url) === hostOf(cfg.base_url))?.id ?? 'custom'
}

// A provider's page. In the Mac window a plain link opens nothing (LESSONS
// #27), so the app opens it in the browser; elsewhere it's a new tab.
function ProviderLink({ p, page, children }: { p: AIProvider; page: 'signup' | 'key'; children: string }) {
  function open(e: MouseEvent) {
    if (!document.documentElement.classList.contains('mac-window')) return
    e.preventDefault()
    api.openProviderPage(p.id, page).catch(() => {})
  }
  return (
    <a className="btn sm" href={page === 'key' ? p.key_page : p.signup} target="_blank" rel="noopener noreferrer" onClick={open}>
      {children}
    </a>
  )
}

// The address, key and model for AI助手. A listed provider fills in the
// address and model and links to where a key is made; "自己填" types all
// three. An empty key keeps the one already saved; the key itself is never
// sent back to the page. Saving checks the connection with one short
// message, and onSaved's ok says whether it went through.
export function AssistantSetup({ cfg, onSaved }: { cfg: AssistantConfig; onSaved: (c: AssistantConfig, ok: boolean) => void }) {
  const { toast } = useApp()
  const providers = cfg.providers || []
  const [pick, setPick] = useState(() => startOn(cfg))
  const [base, setBase] = useState(cfg.base_url)
  const [key, setKey] = useState('')
  const [model, setModel] = useState(() => cfg.model || providers.find((x) => x.id === startOn(cfg))?.models[0]?.id || '')
  // a model that isn't in the provider's list, typed in
  const [other, setOther] = useState(() => {
    const start = providers.find((x) => x.id === startOn(cfg))
    return !!start && !!cfg.model && !start.models.some((m) => m.id === cfg.model)
  })
  const [saved, setSaved] = useState(cfg)
  const [busy, setBusy] = useState<'' | 'save' | 'check'>('')
  const [err, setErr] = useState('')
  const [showLog, setShowLog] = useState(false)
  const [ok, setOk] = useState(false)
  const p = providers.find((x) => x.id === pick)
  const address = p ? p.base_url : base
  // the saved key only goes to the host it was saved for
  const keyKept = saved.has_key && hostOf(address) === hostOf(saved.base_url)

  function choose(id: string) {
    setPick(id)
    setErr('')
    setOk(false)
    const next = providers.find((x) => x.id === id)
    if (!next) return
    setBase(next.base_url)
    const keep = hostOf(saved.base_url) === hostOf(next.base_url) && saved.model ? saved.model : ''
    setModel(keep || next.models[0]?.id || '')
    setOther(!!keep && !next.models.some((m) => m.id === keep))
  }

  async function check(): Promise<boolean> {
    try {
      await api.checkAssistant()
      setOk(true)
      return true
    } catch (e: any) {
      setErr(t('测试没通过：') + e.message)
      return false
    }
  }

  async function save(clear = false) {
    if (busy) return
    if (!clear && p && other && !model.trim()) {
      setErr(t('请填写模型名'))
      return
    }
    setBusy('save')
    setErr('')
    setOk(false)
    try {
      const next = await api.setAssistant({
        base_url: address.trim(),
        model: model.trim(),
        api_key: clear ? '' : key.trim(),
        clear_key: clear,
      })
      setKey('')
      setSaved(next)
      if (!next.configured) {
        onSaved(next, true)
        toast(clear ? t('密钥已忘掉') : t('接口已保存'))
        return
      }
      setBusy('check')
      const good = await check()
      onSaved(next, good)
      if (good) toast(t('接口已保存，连接正常'))
    } catch (e: any) {
      setErr(e.message)
    } finally {
      setBusy('')
    }
  }

  async function test() {
    if (busy) return
    setBusy('check')
    setErr('')
    setOk(false)
    await check()
    setBusy('')
  }

  const name = p ? (isEN() ? p.name_en : p.name) : ''
  const picked = p && !other ? p.models.find((m) => m.id === model) : undefined
  const keyField = (
    <input className="input" type="password" value={key} autoComplete="off" spellCheck={false}
      placeholder={keyKept ? t('已保存密钥，留空表示不改') : t('粘贴密钥')}
      onChange={(e) => setKey(e.target.value)} />
  )

  return (
    <div className="form">
      {providers.length > 0 && (
        <div className="full">
          <div className="seg" role="radiogroup" aria-label={t('服务商')}>
            {providers.map((x) => (
              <button key={x.id} type="button" role="radio" aria-checked={pick === x.id} className={pick === x.id ? 'on' : ''} onClick={() => choose(x.id)}>
                {isEN() ? x.name_en : x.name}
              </button>
            ))}
            <button type="button" role="radio" aria-checked={pick === 'custom'} className={pick === 'custom' ? 'on' : ''} onClick={() => choose('custom')}>
              {t('自己填')}
            </button>
          </div>
        </div>
      )}
      {p ? (
        <>
          <ol className="setup-steps full">
            <li>
              <div className="step-body">
                <span>{t('注册一个{name}账号（已经有了就跳过）', { name })}</span>
                <div className="row"><ProviderLink p={p} page="signup">{t('去注册')}</ProviderLink></div>
                {p.invite && <span className="sub">{t('这个注册链接带着作者的邀请码：你不会多花钱，作者可能拿到平台的奖励。不想用的话，自己打开{name}官网注册也一样。', { name })}</span>}
              </div>
            </li>
            <li>
              <div className="step-body">
                <span>{t('在{name}的 API Key 页面新建一个密钥，复制下来', { name })}</span>
                <div className="row"><ProviderLink p={p} page="key">{t('去创建密钥')}</ProviderLink></div>
              </div>
            </li>
            <li>
              <label className="step-body">
                <span>{t('把密钥粘贴到这里')}</span>
                {keyField}
              </label>
            </li>
          </ol>
          <label className="field full">
            <span className="lbl">{t('模型')}</span>
            <select className="input" value={other ? '' : model} onChange={(e) => {
              const v = e.target.value
              setOther(v === '')
              setModel(v)
            }}>
              {p.models.map((m) => <option key={m.id} value={m.id}>{m.id}</option>)}
              <option value="">{t('其他模型（自己填）')}</option>
            </select>
            {picked && <span className="hint">{isEN() ? picked.note_en : picked.note}</span>}
            <span className="hint">{t('只列出能看图片的模型：AI助手要能读你拍的化验单和处方。')}</span>
          </label>
          {other && (
            <label className="field full">
              <span className="lbl">{t('模型名')}</span>
              <input className="input" value={model} placeholder={t('照服务商文档里的写法填')} autoComplete="off" spellCheck={false}
                onChange={(e) => setModel(e.target.value)} />
              <span className="hint">{t('请填能看图片的模型，不然发图片时会出错。')}</span>
            </label>
          )}
        </>
      ) : (
        <>
          <label className="field full">
            <span className="lbl">{t('接口地址')}</span>
            <input className="input" value={base} placeholder={t('比如 https://api.openai.com/v1')} autoComplete="off" spellCheck={false}
              onChange={(e) => setBase(e.target.value)} />
          </label>
          <label className="field">
            <span className="lbl">{t('密钥')}</span>
            {keyField}
          </label>
          <label className="field">
            <span className="lbl">{t('模型')}</span>
            <input className="input" value={model} placeholder={t('比如 gpt-4o-mini')} autoComplete="off" spellCheck={false}
              onChange={(e) => setModel(e.target.value)} />
          </label>
        </>
      )}
      {err && <div className="error full">{err}</div>}
      {ok && <div className="okay full"><CircleCheck size={16} />{t('连接正常，可以开始用了')}</div>}
      <div className="row full">
        <button type="button" className="btn primary sm" disabled={!!busy} onClick={() => save(false)}>
          {busy === 'save' ? t('正在保存接口…') : busy === 'check' ? t('正在测试连接…') : t('保存并测试')}
        </button>
        {saved.configured && (
          <button type="button" className="btn sm" disabled={!!busy} onClick={test}>{t('测试连接')}</button>
        )}
        {saved.has_key && (
          <button type="button" className="btn sm" disabled={!!busy} onClick={() => save(true)}>{t('忘掉密钥')}</button>
        )}
        <button type="button" className="btn ghost sm" style={{ marginLeft: 'auto' }} onClick={() => setShowLog(true)}>{t('查看日志')}</button>
      </div>
      {showLog && <AssistantLog onClose={() => setShowLog(false)} />}
    </div>
  )
}
