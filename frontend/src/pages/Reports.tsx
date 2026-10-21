import { useLayoutEffect, useRef, useState } from 'react'
import { ClipboardPaste, Search, TestTube2 } from 'lucide-react'
import { api, flagClass, fmtDate, fmtNum } from '../api'
import { LabImport } from '../components/LabImport'
import { LineChart } from '../components/LineChart'
import { LabValue } from '../components/RecordDrawer'
import { t, tr } from '../i18n'
import { hashParam, openRecord, useLoad } from '../state'
import { Empty, KindIcon, RangeBar } from '../ui'
import { RecordsPage } from './RecordsPage'

export function Reports() {
  // #/reports?tab=trends opens on the trends (from the overview's lab card)
  const [tab, setTab] = useState<'reports' | 'trends'>(() => (hashParam('tab') === 'trends' ? 'trends' : 'reports'))
  const [importing, setImporting] = useState(false)
  // each tab comes back scrolled where it was left (the page scrolls, and a
  // short tab would otherwise leave the long one at the top)
  const scrolls = useRef({ reports: 0, trends: 0 })
  const switchTo = (next: 'reports' | 'trends') => {
    if (next === tab) return
    scrolls.current[tab] = window.scrollY
    setTab(next)
  }
  useLayoutEffect(() => {
    window.scrollTo(0, scrolls.current[tab])
  }, [tab])
  const switcher = (
    <div className="seg tab-switch">
      <button className={tab === 'reports' ? 'on' : ''} onClick={() => switchTo('reports')}>{t('报告|tab')}</button>
      <button className={tab === 'trends' ? 'on' : ''} onClick={() => switchTo('trends')}>{t('指标趋势')}</button>
    </div>
  )
  // The switcher is last in the head on both tabs, so it stays put; the
  // reports' own buttons sit to its left. Both tabs stay mounted, so coming
  // back keeps the picked item, the search and the scroll.
  return (
    <>
      <div hidden={tab !== 'reports'}>
        <RecordsPage entityName="reports" end={switcher}
          extra={<button className="btn" onClick={() => setImporting(true)}><ClipboardPaste size={15} />{t('粘贴导入')}</button>} />
      </div>
      <div hidden={tab !== 'trends'}>
        <div className="page-top drag">
          <div className="page-head">
            <div className="titles"><h1>{t('指标趋势')}</h1><div className="sub">{t('每个检验项目的历次结果')}</div></div>
            {switcher}
          </div>
        </div>
        <Trends />
      </div>
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
    return <div className="card"><Empty icon={TestTube2}>{tr('还没有检验指标。导入化验单或体检报告后，{br}这里会显示每项指标的历次变化。', { br: <br /> })}</Empty></div>
  const shown = (items || []).filter((i) => !q || i.item_name.includes(q))
  const exact = trend?.filter((x) => x.item_name === name) || []
  const latest = exact[exact.length - 1]

  return (
    <div className="split">
      <section className="card flush" style={{ padding: '12px 0 6px' }}>
        <div className="input-search" style={{ maxWidth: 'none', margin: '0 12px 6px' }}>
          <Search size={15} />
          <input className="input" placeholder={t('找指标，如 白细胞')} value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div className="list" style={{ maxHeight: '68vh', overflowY: 'auto' }}>
          {shown.map((i) => (
            <div key={i.item_name} className="row-item no-ico" onClick={() => setPick(i.item_name)}
              style={i.item_name === name ? { background: 'var(--accent-soft)' } : undefined}>
              <div className="body">
                <div className="title" style={{ fontWeight: i.item_name === name ? 650 : 500 }}>{i.item_name}</div>
                <div className="meta">{i.panel ? i.panel + ' · ' : ''}{t('{n} 次', { n: i.count })} · {fmtDate(i.latest_date)}</div>
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
              <div className="sub">{t('参考范围 {range}', { range: latest?.ref_range || '—' })} {latest?.unit}</div>
              <h1 style={{ fontSize: 22 }}>{name}</h1>
            </div>
            {latest && <div style={{ textAlign: 'right' }}><div className="sub">{t('最近一次')}</div><LabValue r={latest} /></div>}
          </div>
          {loading && !exact.length ? <Empty compact>{t('加载中…')}</Empty> : (
            <LineChart unit={latest?.unit} low={latest?.ref_low} high={latest?.ref_high}
              points={exact.filter((x) => x.value_num != null).map((x) => ({ date: x.test_date, v: x.value_num, flag: x.flag }))} />
          )}
          <table className="data" style={{ marginTop: 16 }}>
            <thead><tr><th>{t('日期')}</th><th>{t('结果')}</th><th style={{ width: '30%' }} /><th>{t('报告')}</th></tr></thead>
            <tbody>
              {[...exact].reverse().map((x) => (
                <tr key={x.id} className="clickable" onClick={() => x.report_id && openRecord('reports', x.report_id)}>
                  <td className="nowrap">{fmtDate(x.test_date)}</td>
                  <td className="num"><LabValue r={{ ...x, unit: null }} /></td>
                  <td><RangeBar value={x.value_num} low={x.ref_low} high={x.ref_high} flag={x.flag} /></td>
                  <td className="muted ellipsis" style={{ maxWidth: 160 }}>{x.report_title}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </div>
  )
}
