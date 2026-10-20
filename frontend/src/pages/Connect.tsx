import { useState, type ReactNode } from 'react'
import { Check, Copy, Database, Monitor, ShieldCheck, Smartphone, SquareTerminal } from 'lucide-react'
import { api } from '../api'
import { t, tr } from '../i18n'
import { useApp, useLoad } from '../state'
import { IconButton, Switch } from '../ui'

function CodeBox({ text }: { text: string }) {
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

function Section({ icon, title, sub, right, children }: { icon: ReactNode; title: string; sub?: string; right?: ReactNode; children?: ReactNode }) {
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

export function Connect() {
  const { data: info, error, reload } = useLoad(() => api.info(), [])
  const [busy, setBusy] = useState(false)
  if (error) return <div className="error">{error}</div>
  if (!info) return null

  async function toggleLAN(on: boolean) {
    setBusy(true)
    try {
      await api.setLAN(on)
      reload()
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="page-head">
        <div className="titles"><h1>{t('连接 Claude')}</h1><div className="sub">{t('让 Claude 读写你的健康档案，帮你录入和分析')}</div></div>
      </div>
      <div className="stack" style={{ maxWidth: 760 }}>
        <div className="notice">
          <ShieldCheck size={18} />
          <span>
            {t('接好以后可以直接对 Claude 说：「把这张化验单录进去」（附上截图）、「今天吃过药了」、「我近两年的尿酸怎么变化的」。')}{' '}
            {tr('输入框的 {plus} 菜单里还有分析模板：解读最近的化验单、整理就诊摘要、分析症状、健康状况总评、打卡复盘。', { plus: <b>＋</b> })}{' '}
            {t('Claude 读取记录时，相关内容会发送给 Anthropic 处理；数据本身只存在这台电脑上。Claude 的分析不能代替医生诊断。')}
          </span>
        </div>

        <Section icon={<Monitor size={20} />} title={t('Claude 桌面版')} sub={t('设置 → 开发者 → 编辑配置，把下面的内容合并进 claude_desktop_config.json，然后用 ⌘Q 完全退出 Claude 再重新打开。')}>
          <CodeBox text={info.claude_desktop_config} />
        </Section>

        <Section icon={<SquareTerminal size={20} />} title="Claude Code" sub={t('在终端运行一次，对所有项目生效。')}>
          <CodeBox text={info.claude_code_command} />
          <div className="sub" style={{ margin: '14px 0 8px' }}>{t('或者在本 App 运行时，通过 HTTP 连接：')}</div>
          <CodeBox text={info.claude_code_http} />
        </Section>

        <Section icon={<Smartphone size={20} />} title={t('手机访问')}
          sub={t('同一 Wi-Fi 或 Tailscale 网络下的手机，用浏览器打开链接即可记录和打卡；在 Safari 里「添加到主屏幕」后就像一个 App。')}
          right={<Switch checked={info.lan} onChange={(v) => !busy && toggleLAN(v)} label={t('允许手机访问')} />}>
          {info.listen_error && <div className="error">{t('本地服务没有启动：{error}', { error: info.listen_error })}</div>}
          {info.lan && (info.phone_urls?.length
            ? <div className="stack" style={{ gap: 8 }}>{info.phone_urls.map((u) => <CodeBox key={u} text={u} />)}<div className="small muted">{t('链接里带着访问令牌，只发给自己，不要分享给别人。')}</div></div>
            : <div className="sub">{t('没有找到局域网地址，请检查网络连接。')}</div>)}
          {!info.lan && <div className="small muted">{t('手机上的 Claude App 只能连接公网服务器，这部分放在第二期（需要隧道和登录授权）。')}</div>}
        </Section>

        <Section icon={<Database size={20} />} title={t('数据')} sub={t('数据库是 health.db（SQLite），附件在 files/。备份时复制整个目录即可；建议开启 FileVault 磁盘加密。')}>
          <dl className="kv" style={{ boxShadow: 'none', padding: 0, background: 'none' }}>
            <div className="kv-row"><dt>{t('数据目录')}</dt><dd><code>{info.data_dir}</code></dd></div>
            <div className="kv-row"><dt>{t('本地服务')}</dt><dd>{info.listening || t('未启动')}</dd></div>
            <div className="kv-row"><dt>{t('版本')}</dt><dd>{info.version}</dd></div>
          </dl>
        </Section>
      </div>
    </>
  )
}
