import { useEffect, useState } from 'react'
import { MoreHorizontal, Search as SearchIcon, Settings as SettingsIcon } from 'lucide-react'
import { api, type Overview as OV } from './api'
import { t, tx, type Key } from './i18n'
import { RecordDrawer } from './components/RecordDrawer'
import { Checkins } from './pages/Checkins'
import { Connect } from './pages/Connect'
import { Metrics } from './pages/Metrics'
import { Overview } from './pages/Overview'
import { RecordList, RecordsPage } from './pages/RecordsPage'
import { Reports } from './pages/Reports'
import { Settings } from './pages/Settings'
import { useApp, useLoad, useRoute } from './state'
import { Empty, KindIcon, KIND } from './ui'

const NAV: { group: Key | ''; items: { page: string; label: Key; count?: string }[] }[] = [
  { group: '', items: [{ page: 'overview', label: '今天' }, { page: 'checkins', label: '打卡' }] },
  {
    group: '记录',
    items: [
      { page: 'conditions', label: '健康问题', count: 'conditions' },
      { page: 'symptoms', label: '症状日志', count: 'symptoms' },
      { page: 'visits', label: '就诊', count: 'visits' },
      { page: 'medications', label: '用药', count: 'medications' },
    ],
  },
  {
    group: '数据',
    items: [
      { page: 'reports', label: '检查报告', count: 'reports' },
      { page: 'metrics', label: '身体数据', count: 'metrics' },
    ],
  },
]
const TABS = ['overview', 'checkins', 'reports', 'metrics']
const LABEL: Record<string, Key> = Object.fromEntries(NAV.flatMap((g) => g.items.map((i) => [i.page, i.label])))
LABEL.connect = '连接 AI'
LABEL.settings = '设置'
LABEL.search = '搜索'

export function App() {
  const [route, go] = useRoute()
  const { version } = useApp()
  const [counts, setCounts] = useState<OV['counts']>({})
  const [q, setQ] = useState('')
  const [more, setMore] = useState(false)
  useEffect(() => {
    api.overview().then((o) => setCounts(o.counts)).catch(() => {})
  }, [version])
  useEffect(() => setMore(false), [route.page])

  const page = route.page
  let body
  switch (page) {
    case 'checkins': body = <Checkins />; break
    case 'reports': body = <Reports />; break
    case 'metrics': body = <Metrics />; break
    case 'connect': body = <Connect />; break
    case 'settings': case 'backup': body = <Settings />; break
    case 'search': body = <Search q={q} setQ={setQ} />; break
    case 'conditions': case 'symptoms': case 'visits': case 'medications':
      body = <RecordsPage key={page} entityName={page} />; break
    default: body = <Overview />
  }

  const navItem = (p: string, label: Key, count?: string) => (
    <a key={p} href={'#/' + p} className={'nav-item' + (page === p ? ' active' : '')}>
      <KindIcon kind={p} size="sm" />{t(label)}
      {count && counts[count] ? <span className="count">{counts[count]}</span> : null}
    </a>
  )


  // the settings sit behind a gear at the end of the 连接 AI row
  const settingsLink = (
    <a href="#/settings" className={'icon-btn' + (page === 'settings' ? ' on' : '')} aria-label={t('设置')} title={t('设置')}>
      <SettingsIcon size={18} />
    </a>
  )

  return (
    <div className="app">
      <nav className="sidebar drag">
        <div className="brand"><img src="/icon.svg" alt="" />{t('健康档案')}</div>
        <div className="side-search">
          <SearchIcon size={14} />
          <input placeholder={t('搜索')} value={q}
            onChange={(e) => { setQ(e.target.value); if (e.target.value && page !== 'search') go({ page: 'search' }) }} />
        </div>
        {NAV.map((g) => (
          <div className="nav-group" key={g.group}>
            {g.group && <div className="nav-label">{t(g.group)}</div>}
            {g.items.map((i) => navItem(i.page, i.label, i.count))}
          </div>
        ))}
        <div className="side-foot foot-row">{navItem('connect', '连接 AI')}{settingsLink}</div>
      </nav>
      <div className="main-wrap">
        <div className="drag-strip drag" />
        <main className="main">{body}</main>
      </div>

      <nav className="tabbar">
        {TABS.map((p) => {
          const Icon = KIND[p].icon
          return (
            <a key={p} href={'#/' + p} className={'tab' + (page === p ? ' active' : '')}>
              <Icon size={22} strokeWidth={page === p ? 2.3 : 1.8} />{t(LABEL[p])}
            </a>
          )
        })}
        <button className={'tab' + (!TABS.includes(page) ? ' active' : '')} onClick={() => setMore(true)}>
          <MoreHorizontal size={22} />{t('更多')}
        </button>
      </nav>
      <div className={'more-sheet' + (more ? ' open' : '')} onClick={() => setMore(false)}>
        <div className="panel" onClick={(e) => e.stopPropagation()}>
          {['conditions', 'symptoms', 'visits', 'medications', 'search'].map((p) =>
            navItem(p, LABEL[p], NAV.flatMap((g) => g.items).find((i) => i.page === p)?.count))}
          <div className="foot-row">{navItem('connect', '连接 AI')}{settingsLink}</div>
        </div>
      </div>

      {route.open && <RecordDrawer key={route.open.entity + route.open.id} entityName={route.open.entity} id={route.open.id} />}
    </div>
  )
}

function Search({ q, setQ }: { q: string; setQ: (q: string) => void }) {
  const { entity } = useApp()
  const { data, error } = useLoad(() => (q.trim() ? api.search(q.trim()) : Promise.resolve([])), [q])
  return (
    <>
      <div className="page-head">
        <div className="titles"><h1>{t('搜索')}</h1></div>
      </div>
      <div className="input-search" style={{ maxWidth: 480, marginBottom: 20 }}>
        <SearchIcon size={15} />
        <input className="input" autoFocus placeholder={t('搜索所有记录，如 胃、头孢、协和')} value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {error && <div className="error">{error}</div>}
      {data?.length === 0 && q && <Empty icon={SearchIcon}>{t('没有找到「{q}」', { q })}</Empty>}
      <div className="stack">
        {data?.map((h) => (
          <section key={h.type}>
            <div className="section-title">{tx(h.type_label)} · {h.total}</div>
            <div className="card flush"><RecordList entity={entity(h.type)} rows={h.rows} /></div>
          </section>
        ))}
      </div>
    </>
  )
}
