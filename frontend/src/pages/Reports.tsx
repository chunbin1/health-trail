import { useState } from 'react'
import { ClipboardPaste, Search, TestTube2 } from 'lucide-react'
import { api, flagClass, fmtDate, fmtNum } from '../api'
import { LabImport } from '../components/LabImport'
import { LineChart } from '../components/LineChart'
import { LabValue } from '../components/RecordDrawer'
import { openRecord, useLoad } from '../state'
import { Empty, KindIcon, RangeBar } from '../ui'
import { RecordsPage } from './RecordsPage'

export function Reports() {
  const [tab, setTab] = useState<'reports' | 'trends'>('reports')
  const [importing, setImporting] = useState(false)
  const switcher = (
    <div className="seg">
      <button className={tab === 'reports' ? 'on' : ''} onClick={() => setTab('reports')}>报告</button>
      <button className={tab === 'trends' ? 'on' : ''} onClick={() => setTab('trends')}>指标趋势</button>
    </div>
  )
  return (
    <>
      {tab === 'reports' ? (
        <RecordsPage entityName="reports" extra={<>{switcher}<button className="btn" onClick={() => setImporting(true)}><ClipboardPaste size={15} />粘贴导入</button></>} />
      ) : (
        <>
          <div className="page-head">
            <div className="titles"><h1>指标趋势</h1><div className="sub">每个检验项目的历次结果</div></div>
            {switcher}
          </div>
          <Trends />
        </>
      )}
      {importing && <LabImport onClose={() => setImporting(false)} onSaved={(id) => { setImporting(false); openRecord('reports', id) }} />}
    </>
  )
}

function Trends() {
  const { data: items } = useLoad(() => api.labItems(), [])
  const [q, setQ] = useState('')
  const [pick, setPick] = useState('')
  const name = pick || items?.[0]?.item_name || ''
  const { data: trend, loading } = useLoad(() => (name ? api.labTrend(name) : Promise.resolve([])), [name])

  if (items?.length === 0)
    return <div className="card"><Empty icon={TestTube2}>还没有检验指标。导入化验单或体检报告后，<br />这里会显示每项指标的历次变化。</Empty></div>
  const shown = (items || []).filter((i) => !q || i.item_name.includes(q))
  const exact = trend?.filter((t) => t.item_name === name) || []
  const latest = exact[exact.length - 1]

  return (
    <div className="split">
      <section className="card" style={{ padding: '12px 0 6px' }}>
        <div className="input-search" style={{ maxWidth: 'none', margin: '0 12px 6px' }}>
          <Search size={15} />
          <input className="input" placeholder="找指标，如 白细胞" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div className="list" style={{ maxHeight: '68vh', overflowY: 'auto' }}>
          {shown.map((i) => (
            <div key={i.item_name} className="row-item no-ico" onClick={() => setPick(i.item_name)}
              style={i.item_name === name ? { background: 'var(--accent-soft)' } : undefined}>
              <div className="body">
                <div className="title" style={{ fontWeight: i.item_name === name ? 650 : 500 }}>{i.item_name}</div>
                <div className="meta">{i.panel ? i.panel + ' · ' : ''}{i.count} 次 · {fmtDate(i.latest_date)}</div>
              </div>
              <span className={'val ' + flagClass(i.latest_flag)}>{fmtNum(i.latest_value)}</span>
            </div>
          ))}
        </div>
      </section>
      {name && (
        <section className="card">
          <div className="drawer-title" style={{ marginBottom: 16 }}>
            <KindIcon kind="lab_results" size="lg" />
            <div style={{ flex: 1, minWidth: 0 }}>
              <div className="sub">参考范围 {latest?.ref_range || '—'} {latest?.unit}</div>
              <h1 style={{ fontSize: 22 }}>{name}</h1>
            </div>
            {latest && <div style={{ textAlign: 'right' }}><div className="sub">最近一次</div><LabValue r={latest} /></div>}
          </div>
          {loading && !exact.length ? <Empty compact>加载中…</Empty> : (
            <LineChart unit={latest?.unit} low={latest?.ref_low} high={latest?.ref_high}
              points={exact.filter((t) => t.value_num != null).map((t) => ({ date: t.test_date, v: t.value_num, flag: t.flag }))} />
          )}
          <table className="data" style={{ marginTop: 16 }}>
            <thead><tr><th>日期</th><th>结果</th><th style={{ width: '30%' }} /><th>报告</th></tr></thead>
            <tbody>
              {[...exact].reverse().map((t) => (
                <tr key={t.id} className="clickable" onClick={() => t.report_id && openRecord('reports', t.report_id)}>
                  <td className="nowrap">{fmtDate(t.test_date)}</td>
                  <td className="num"><LabValue r={{ ...t, unit: null }} /></td>
                  <td><RangeBar value={t.value_num} low={t.ref_low} high={t.ref_high} flag={t.flag} /></td>
                  <td className="muted ellipsis" style={{ maxWidth: 160 }}>{t.report_title}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </div>
  )
}
