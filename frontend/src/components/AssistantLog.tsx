import { useState } from 'react'
import { ChevronDown, ChevronRight, ScrollText } from 'lucide-react'
import { api, type AssistantLogEntry } from '../api'
import { t } from '../i18n'
import { useLoad } from '../state'
import { Modal } from './Modal'

// The assistant's log: each reply as it went, newest first. Read from
// assistant.log in the data folder (the end of it when it's big); it is
// written in Chinese, the way the model was told things.
export function AssistantLog({ onClose }: { onClose: () => void }) {
  const { data, error, loading, reload } = useLoad(() => api.assistantLog(), [])
  const [onlyFailed, setOnlyFailed] = useState(false)
  // the newest reply starts open; the rest are opened by hand
  const [open, setOpen] = useState<Record<number, boolean>>({ 0: true })
  const entries = (data?.entries || []).map((e, i) => ({ e, i })).filter(({ e }) => !onlyFailed || e.failed)

  return (
    <Modal title={t('AI助手日志')} icon={<ScrollText size={20} />} onClose={onClose} wide>
      <div className="modal-body assist-log">
        <p className="sub" style={{ marginTop: 0 }}>
          {t('每次回答的经过：问了什么、调用了哪些工具、哪里出错。最近的在最前面。日志只存在这台电脑的数据目录里（assistant.log），不含密钥。')}
          {data?.cut && ' ' + t('日志比较长，这里只显示最后一部分。')}
        </p>
        <div className="pills" style={{ marginBottom: 12 }}>
          <button type="button" className={'pill' + (!onlyFailed ? ' on' : '')} onClick={() => setOnlyFailed(false)}>{t('全部')}</button>
          <button type="button" className={'pill' + (onlyFailed ? ' on' : '')} onClick={() => setOnlyFailed(true)}>{t('只看出错的')}</button>
        </div>
        {error && <div className="error">{error}</div>}
        {!data && loading && <div className="sub">{t('加载中…')}</div>}
        {data && entries.length === 0 && (
          <div className="sub">{onlyFailed ? t('最近没有出错的回答') : t('还没有日志。用 AI助手问一次之后，这里会记下经过。')}</div>
        )}
        <div className="log-list">
          {entries.map(({ e, i }) => (
            <LogItem key={i + e.at} entry={e} open={!!open[i]} onToggle={() => setOpen({ ...open, [i]: !open[i] })} />
          ))}
        </div>
      </div>
      <div className="modal-foot">
        <span className="small muted" style={{ marginRight: 'auto' }}>
          {data ? t('共 {n} 次回答', { n: data.entries.length }) : ''}
        </span>
        <button type="button" className="btn sm" disabled={loading} onClick={reload}>{loading ? t('正在读取…') : t('刷新')}</button>
      </div>
    </Modal>
  )
}

function LogItem({ entry: e, open, onToggle }: { entry: AssistantLogEntry; open: boolean; onToggle: () => void }) {
  return (
    <div className={'log-item' + (e.failed ? ' failed' : '')}>
      <button type="button" className="log-head" aria-expanded={open} onClick={onToggle}>
        {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        <span className="log-ask ellipsis">{e.ask || t('（图片）')}</span>
        {e.failed && <span className="tag bad">{t('出错')}</span>}
        <span className="log-meta small muted">
          {[e.at.slice(5, 16), e.model, e.requests ? t('{n} 次请求', { n: e.requests }) : '', e.tokens ? t('{n} tokens', { n: e.tokens }) : '']
            .filter(Boolean).join(' · ')}
        </span>
      </button>
      {open && (
        <pre className="log-text">
          {e.text.split('\n').map((line, k) => (
            <span key={k} className={lineClass(line)}>{line + '\n'}</span>
          ))}
        </pre>
      )}
    </div>
  )
}

function lineClass(line: string) {
  if (line.startsWith('调用 ')) return 'call'
  if (line.startsWith('  → 出错：') || line === '--- 出错 ---') return 'err'
  if (line.startsWith('--- ') || line.startsWith('tokens 合计') || line.startsWith('每次请求都带')) return 'sec'
  if (line.startsWith('  → ') || line.startsWith('    ')) return 'out'
  return undefined
}
