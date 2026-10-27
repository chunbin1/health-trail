import { useState } from 'react'
import { api, type AssistantConfig } from '../api'
import { t } from '../i18n'
import { useApp } from '../state'

// The address, key and model for AI助手. An empty key keeps the one already
// saved; the key itself is never sent back to the page.
export function AssistantSetup({ cfg, onSaved }: { cfg: AssistantConfig; onSaved: (c: AssistantConfig) => void }) {
  const { toast } = useApp()
  const [base, setBase] = useState(cfg.base_url)
  const [key, setKey] = useState('')
  const [model, setModel] = useState(cfg.model)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  async function save(clear = false) {
    if (busy) return
    setBusy(true)
    setErr('')
    try {
      const next = await api.setAssistant({
        base_url: base.trim(),
        model: model.trim(),
        api_key: clear ? '' : key.trim(),
        clear_key: clear,
      })
      setKey('')
      onSaved(next)
      toast(t('接口已保存'))
    } catch (e: any) {
      setErr(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="form">
      <label className="field full">
        <span className="lbl">{t('接口地址')}</span>
        <input className="input" value={base} placeholder={t('比如 https://api.openai.com/v1')} autoComplete="off" spellCheck={false}
          onChange={(e) => setBase(e.target.value)} />
      </label>
      <label className="field">
        <span className="lbl">{t('密钥')}</span>
        <input className="input" type="password" value={key} autoComplete="off" spellCheck={false}
          placeholder={cfg.has_key ? t('已保存密钥，留空表示不改') : ''}
          onChange={(e) => setKey(e.target.value)} />
      </label>
      <label className="field">
        <span className="lbl">{t('模型')}</span>
        <input className="input" value={model} placeholder={t('比如 gpt-4o-mini')} autoComplete="off" spellCheck={false}
          onChange={(e) => setModel(e.target.value)} />
      </label>
      {err && <div className="error full">{err}</div>}
      <div className="row full">
        <button type="button" className="btn primary sm" disabled={busy} onClick={() => save(false)}>
          {busy ? t('正在保存接口…') : t('保存接口')}
        </button>
        {cfg.has_key && (
          <button type="button" className="btn sm" disabled={busy} onClick={() => save(true)}>{t('忘掉密钥')}</button>
        )}
      </div>
    </div>
  )
}
