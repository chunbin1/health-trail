import { useState } from 'react'
import { ArrowLeft, ClipboardPaste, Sparkles, X } from 'lucide-react'
import { api, flagClass, today, type LabRow, type Rec } from '../api'
import { t, tr, tx } from '../i18n'
import { useApp } from '../state'
import { IconButton, KindIcon } from '../ui'
import { DateTimeInput } from './DateTimeInput'
import { Modal } from './Modal'

const FLAGS = ['', '正常', '偏高', '偏低', '异常']
const TYPES = ['体检', '化验', '影像', '病理', '其他']

// Paste a lab report's table, check the parsed rows, save them — into the
// given report, or into a new one described at the top.
export function LabImport({ report, onClose, onSaved }: { report?: Rec; onClose: () => void; onSaved: (reportId: number) => void }) {
  const { changed, toast } = useApp()
  const [text, setText] = useState('')
  const [rows, setRows] = useState<(LabRow & { value: string })[] | null>(null)
  const [meta, setMeta] = useState({ report_date: today(), title: '', report_type: '化验', institution: '' })
  const [panel, setPanel] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function parse() {
    setError('')
    try {
      const parsed = await api.labParse(text)
      if (parsed.length === 0) {
        setError(t('没有识别出指标。每行应包含：项目名、结果、单位（可省略）、参考范围（可省略）。'))
        return
      }
      setRows(parsed.map((r) => ({ ...r, value: r.value_text || (r.value_num != null ? String(r.value_num) : '') })))
    } catch (e: any) {
      setError(e.message)
    }
  }

  function edit(i: number, k: string, v: string) {
    setRows((rs) => rs!.map((r, j) => (j === i ? { ...r, [k]: v } : r)))
  }

  async function save() {
    setBusy(true)
    setError('')
    try {
      const body = rows!.map((r) => {
        const isNum = /^-?\d+(\.\d+)?$/.test(r.value.trim())
        return {
          item_name: r.item_name, unit: r.unit, ref_range: r.ref_range, flag: r.flag || '',
          ...(isNum ? { value_num: r.value.trim() } : { value_text: r.value }),
        }
      })
      const res = await api.labImport(report ? { report_id: report.id, panel, rows: body } : { report: meta, panel, rows: body })
      changed()
      toast(t('已保存 {n} 项指标', { n: res.lab_results.length }))
      onSaved(res.report.id)
    } catch (e: any) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  const abnormal = rows?.filter((r) => r.flag && r.flag !== '正常').length ?? 0

  return (
    <Modal title={report ? t('导入指标到「{title}」', { title: report.title }) : t('粘贴导入化验单')} icon={<KindIcon kind="reports" />} onClose={onClose} wide>
      <div className="modal-body">
        <div className="stack" style={{ gap: 16 }}>
          {!report && (
            <div className="form">
              <label className="field">
                <span className="lbl">{t('标题')}<span className="req">*</span></span>
                <input className="input" value={meta.title} placeholder={t('如：血常规+CRP')} onChange={(e) => setMeta({ ...meta, title: e.target.value })} />
              </label>
              <label className="field">
                <span className="lbl">{t('报告日期')}<span className="req">*</span></span>
                <DateTimeInput mode="date" required value={meta.report_date} onChange={(report_date) => setMeta({ ...meta, report_date })} />
              </label>
              <div className="field">
                <span className="lbl">{t('类型')}</span>
                <div className="seg">
                  {TYPES.map((o) => <button type="button" key={o} className={meta.report_type === o ? 'on' : ''} onClick={() => setMeta({ ...meta, report_type: o })}>{tx(o)}</button>)}
                </div>
              </div>
              <label className="field">
                <span className="lbl">{t('机构')}</span>
                <input className="input" value={meta.institution} placeholder={t('医院或体检中心')} onChange={(e) => setMeta({ ...meta, institution: e.target.value })} />
              </label>
            </div>
          )}
          <label className="field">
            <span className="lbl">{t('分组（可选）')}</span>
            <input className="input" value={panel} placeholder={t('如：血常规、肝功能')} onChange={(e) => setPanel(e.target.value)} style={{ maxWidth: 320 }} />
          </label>
          {!rows ? (
            <>
              <textarea className="input" style={{ minHeight: 220, fontFamily: 'var(--mono)', fontSize: 13 }}
                placeholder={t('把化验单的表格文字粘贴到这里，例如：\n\n超敏C-反应蛋白\t111.87↑\tmg/L\t0-6\n白细胞\t17.62↑\t10^9/L\t3.5-9.5\n\n从手机复制的「一格一行」格式也可以。')}
                value={text} onChange={(e) => setText(e.target.value)} />
              <div className="notice"><Sparkles size={16} /><span>{t('只有截图或 PDF？直接发给 AI，说「帮我把这份化验单录进健康档案」。')}</span></div>
            </>
          ) : (
            <>
              <div className="notice">
                <ClipboardPaste size={16} />
                <span>{tr('识别出 {n} 项，其中 {abnormal} 项异常。请对照原报告核对，可以直接修改。', { n: <b>{rows.length}</b>, abnormal: <b className={abnormal ? 'flag-high' : ''}>{abnormal}</b> })}</span>
              </div>
              <div className="table-wrap">
                <table className="data">
                  <thead><tr><th>{t('项目')}</th><th>{t('结果')}</th><th>{t('单位')}</th><th>{t('参考范围')}</th><th>{t('标记')}</th><th /></tr></thead>
                  <tbody>
                    {rows.map((r, i) => (
                      <tr key={i}>
                        <td><input className="input" value={r.item_name} onChange={(e) => edit(i, 'item_name', e.target.value)} /></td>
                        <td style={{ width: 110 }}><input className={'input num ' + flagClass(r.flag)} value={r.value} onChange={(e) => edit(i, 'value', e.target.value)} /></td>
                        <td style={{ width: 100 }}><input className="input" value={r.unit || ''} onChange={(e) => edit(i, 'unit', e.target.value)} /></td>
                        <td style={{ width: 120 }}><input className="input num" value={r.ref_range || ''} onChange={(e) => edit(i, 'ref_range', e.target.value)} /></td>
                        <td style={{ width: 96 }}>
                          <select className={'input ' + flagClass(r.flag)} value={r.flag || ''} onChange={(e) => edit(i, 'flag', e.target.value)}>
                            {FLAGS.map((f) => <option key={f} value={f}>{f ? tx(f) : t('自动')}</option>)}
                          </select>
                        </td>
                        <td style={{ width: 32 }}><IconButton label={t('删除这一行')} small onClick={() => setRows(rows.filter((_, j) => j !== i))}><X size={15} /></IconButton></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
          {error && <div className="error">{error}</div>}
        </div>
      </div>
      <div className="modal-foot">
        {rows && <button className="btn ghost" onClick={() => setRows(null)}><ArrowLeft size={15} />{t('返回修改')}</button>}
        <span className="spacer" />
        <button className="btn" onClick={onClose}>{t('取消')}</button>
        {!rows ? (
          <button className="btn primary" disabled={!text.trim()} onClick={parse}>{t('识别')}</button>
        ) : (
          <button className="btn primary" disabled={busy || rows.length === 0 || (!report && !meta.title)} onClick={save}>
            {busy ? t('保存中…') : t('保存 {n} 项', { n: rows.length })}
          </button>
        )}
      </div>
    </Modal>
  )
}
