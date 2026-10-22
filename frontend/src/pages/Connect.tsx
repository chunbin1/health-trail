import { useState, type ReactNode } from 'react'
import { Check, Copy, MessageSquare, Plug, ShieldCheck, SquareTerminal } from 'lucide-react'
import { api } from '../api'
import { t, tr } from '../i18n'
import { useApp, useLoad } from '../state'
import { IconButton } from '../ui'

export function CodeBox({ text }: { text: string }) {
  const { toast } = useApp()
  const [done, setDone] = useState(false)
  return (
    <div className="code-box">
      <pre className="code" style={{ paddingRight: 48 }}>{text}</pre>
      <IconButton label={t('复制')} small onClick={() => navigator.clipboard.writeText(text).then(
        () => { setDone(true); toast(t('已复制')); setTimeout(() => setDone(false), 1500) },
        () => toast(t('复制失败，请手动选择')))}>
        {done ? <Check size={14} /> : <Copy size={14} />}
      </IconButton>
    </div>
  )
}

export function Section({ icon, title, sub, right, children }: { icon: ReactNode; title: string; sub?: string; right?: ReactNode; children?: ReactNode }) {
  return (
    <section className="card">
      <div className="drawer-title" style={{ marginBottom: children ? 14 : 0, alignItems: 'center' }}>
        <span className="chip-ico lg" style={{ ['--k' as any]: 'var(--ink-2)' }}>{icon}</span>
        <div style={{ flex: 1, minWidth: 0 }}>
          <h2>{title}</h2>
          {sub && <div className="sub" style={{ marginTop: 2 }}>{sub}</div>}
        </div>
        {right}
      </div>
      {children}
    </section>
  )
}

// The AIs this page has a setup for; the person picks theirs and sees only
// that one. The pick is remembered in this browser.
const AIS = [
  { id: 'claude-chat', label: 'Claude 聊天' },
  { id: 'claude-code', label: 'Claude Code' },
  { id: 'codex', label: 'Codex' },
  { id: 'other', label: '其他 AI' },
] as const
type AI = (typeof AIS)[number]['id']
const AI_KEY = 'health-trail.ai'
const OLD_AI_KEY = 'health-vault.ai' // before the app was called Health Trail

function storedAI(): AI {
  try {
    const v = localStorage.getItem(AI_KEY) ?? localStorage.getItem(OLD_AI_KEY)
    if (AIS.some((a) => a.id === v)) return v as AI
  } catch {}
  return 'claude-chat'
}

export function Connect() {
  const { data: info, error } = useLoad(() => api.info(), [])
  const [ai, setAI] = useState<AI>(storedAI)
  if (error) return <div className="error">{error}</div>
  if (!info) return null

  function pick(a: AI) {
    setAI(a)
    try {
      localStorage.setItem(AI_KEY, a)
    } catch {}
  }

  return (
    <>
      <div className="page-head">
        <div className="titles"><h1>{t('连接 AI')}</h1><div className="sub">{t('让 Claude、Codex 等 AI 读写你的健康档案，帮你录入和分析')}</div></div>
      </div>
      <div className="stack" style={{ maxWidth: 760 }}>
        <div className="notice">
          <ShieldCheck size={18} />
          <span>
            {t('接好以后可以直接对 AI 说：「把这张化验单录进去」（附上截图）、「今天吃过药了」、「我近两年的尿酸怎么变化的」。')}{' '}
            {tr('Claude 聊天输入框的 {plus} 菜单里还有分析模板：解读最近的化验单、整理就诊摘要、分析症状、健康状况总评、打卡复盘。', { plus: <b>＋</b> })}{' '}
            {t('AI 读取记录时，相关内容会发送给它的服务商处理（Claude 发给 Anthropic，Codex 发给 OpenAI）；数据本身只存在这台电脑上。AI 的分析不能代替医生诊断。')}
          </span>
        </div>

        <div>
          <div className="sub" style={{ marginBottom: 8 }}>{t('你用的是哪个 AI？')}</div>
          <div className="seg" role="radiogroup" aria-label={t('你用的是哪个 AI？')}>
            {AIS.map((a) => (
              <button key={a.id} type="button" role="radio" aria-checked={ai === a.id} className={ai === a.id ? 'on' : ''} onClick={() => pick(a.id)}>
                {a.id === 'claude-chat' || a.id === 'other' ? t(a.label) : a.label}
              </button>
            ))}
          </div>
        </div>

        {ai === 'claude-chat' && (
          <Section icon={<MessageSquare size={20} />} title={t('Claude 聊天')}
            sub={t('Claude 桌面 App 里的普通对话。在 Claude 的「设置 → 开发者 → 编辑配置」里，把下面的内容合并进 claude_desktop_config.json，然后用 ⌘Q 完全退出 Claude 再重新打开。')}>
            <CodeBox text={info.claude_desktop_config} />
          </Section>
        )}

        {ai === 'claude-code' && (
          <Section icon={<SquareTerminal size={20} />} title="Claude Code"
            sub={t('终端里的 claude 命令和 Claude 桌面 App 的「Code」页用的是同一份设置。在终端运行一次，对所有项目生效。')}>
            <CodeBox text={info.claude_code_command} />
            <div className="sub" style={{ margin: '14px 0 8px' }}>{t('或者在本 App 运行时，通过 HTTP 连接：')}</div>
            <CodeBox text={info.claude_code_http} />
          </Section>
        )}

        {ai === 'codex' && (
          <Section icon={<SquareTerminal size={20} />} title="Codex" sub={t('在终端运行一次；Codex 命令行和编辑器插件共用这个设置。')}>
            <CodeBox text={info.codex_command} />
            <div className="sub" style={{ margin: '14px 0 8px' }}>{t('或者手动加进 ~/.codex/config.toml：')}</div>
            <CodeBox text={info.codex_config} />
            <div className="sub" style={{ margin: '14px 0 8px' }}>{t('或者在本 App 运行时，通过 HTTP 连接：')}</div>
            <CodeBox text={info.codex_http} />
          </Section>
        )}

        {ai === 'other' && (
          <Section icon={<Plug size={20} />} title={t('其他支持 MCP 的 AI')}
            sub={t('Cursor、Cherry Studio 等：添加一个 MCP 服务器，类型选「命令行（stdio）」，填下面的命令和参数；或者类型选 HTTP，填地址（需要本 App 开着）。')}>
            <div className="stack" style={{ gap: 8 }}>
              <div className="sub">{t('命令')}</div>
              <CodeBox text={info.mcp_stdio_command} />
              <div className="sub">{t('参数')}</div>
              <CodeBox text={info.mcp_args} />
              <div className="sub">{t('HTTP 地址')}</div>
              <CodeBox text={info.mcp_http_url} />
            </div>
          </Section>
        )}
      </div>
    </>
  )
}
