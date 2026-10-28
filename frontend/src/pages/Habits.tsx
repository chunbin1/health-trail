import { ChevronLeft } from 'lucide-react'
import { addDays, api, fmtDate, today, type HabitStats, type Rec } from '../api'
import { tell } from '../components/Dialog'
import { t } from '../i18n'
import { openRecord, useApp, useLoad } from '../state'
import { Empty, habitIcon } from '../ui'

// Every check-in item, in use and archived, with all its check-ins: the one
// place to archive an item (it's never deleted) and to bring one back. The
// 打卡 page shows only the items in use.
export function AllHabits() {
  const { changed, toast } = useApp()
  const { data: list } = useLoad(() => api.list('habits', { limit: 1000 }), [])
  // all time: an archived item's check-ins are kept, and shown here
  const { data: stats } = useLoad(() => api.checkinStats('2000-01-01', today()), [])
  const from = addDays(today(), -29)
  const days = Array.from({ length: 30 }, (_, i) => addDays(from, i))
  const byId = new Map<number, HabitStats>((stats || []).map((s) => [s.habit.id, s]))
  const all = [...(list?.rows || [])].sort((a, b) => a.id - b.id)
  const inUse = all.filter((h) => !h.archived)
  const archived = all.filter((h) => !!h.archived)

  async function setArchived(h: Rec, on: boolean) {
    const name = `${habitIcon(h)} ${h.name}`
    try {
      await api.update('habits', h.id, { archived: on })
      changed()
      toast(on ? t('已归档「{name}」：以后不用再打卡，打过的记录都还在', { name }) : t('已恢复「{name}」', { name }))
    } catch (e: any) {
      tell(e.message)
    }
  }

  const row = (h: Rec) => {
    const s = byId.get(h.id)
    const dates = s?.dates || []
    const last = dates[dates.length - 1]
    return (
      <div key={h.id} className="habit-row">
        <a href="#" onClick={(e) => { e.preventDefault(); openRecord('habits', h.id) }} className="habit-name">
          <span style={{ fontSize: 18 }}>{habitIcon(h)}</span>
          <span style={{ minWidth: 0 }}>
            <div className="ellipsis" style={{ fontWeight: 600 }}>{h.name}</div>
            <div className="muted small ellipsis">
              {dates.length ? t('共打卡 {n} 天', { n: dates.length }) + ' · ' + t('最近一次 {date}', { date: fmtDate(last) }) : t('还没打过卡')}
            </div>
          </span>
        </a>
        <div className="heat" aria-label={t('{name}最近30天完成{n}天', { name: h.name, n: dates.filter((d) => d >= from).length })}>
          {days.map((d) => (
            <span key={d} title={`${fmtDate(d)}${dates.includes(d) ? ' ✓' : ''}`} className={(dates.includes(d) ? 'on' : '') + (d === today() ? ' today' : '')} />
          ))}
        </div>
        <button className="btn sm habit-act" onClick={() => setArchived(h, !h.archived)}>{h.archived ? t('恢复') : t('归档')}</button>
      </div>
    )
  }

  return (
    <>
      <div className="page-head">
        <div className="titles">
          <a href="#/checkins" className="back-link"><ChevronLeft size={16} />{t('打卡')}</a>
          <h1>{t('全部打卡项')}</h1>
          <div className="sub">{t('不再打卡的打卡项可以归档：打卡页上就不显示了，打过的卡都留在这里；恢复后又会回到打卡页。')}</div>
        </div>
      </div>
      {list && stats && (
        <div className="stack">
          <section className="card">
            <div className="card-head"><h2>{t('在用')}</h2><span className="muted small">{inUse.length}</span></div>
            {inUse.length === 0 ? <Empty compact>{t('没有在用的打卡项')}</Empty> : <div className="habit-rows all">{inUse.map(row)}</div>}
          </section>
          <section className="card">
            <div className="card-head"><h2>{t('已归档')}</h2><span className="muted small">{archived.length}</span></div>
            {archived.length === 0 ? <Empty compact>{t('还没有归档的打卡项')}</Empty> : <div className="habit-rows all">{archived.map(row)}</div>}
          </section>
        </div>
      )}
    </>
  )
}
