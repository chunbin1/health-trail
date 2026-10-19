import { useState } from 'react'
import { ChevronLeft, ChevronRight, CircleCheck, Circle, Flame, Plus } from 'lucide-react'
import { addDays, api, fmtDate, fmtNum, today, type HabitDay } from '../api'
import { EntityForm } from '../components/EntityForm'
import { Modal } from '../components/Modal'
import { openRecord, useApp, useLoad } from '../state'
import { Empty, IconButton, KindIcon } from '../ui'

// The day's habits as buttons: one tap checks in (or asks for the amount,
// for a habit with a unit); a tap on a done habit undoes it.
export function HabitGrid({ days, date }: { days: HabitDay[]; date: string }) {
  const { changed, toast } = useApp()
  const [asking, setAsking] = useState<HabitDay | null>(null)
  const [amount, setAmount] = useState('')

  async function tap(d: HabitDay) {
    try {
      if (d.checkin) {
        await api.uncheck(d.habit.id, date)
      } else if (d.habit.unit) {
        setAmount(d.habit.target != null ? String(d.habit.target) : '')
        setAsking(d)
        return
      } else {
        await api.checkin(d.habit.id, date)
        toast(`${d.habit.icon || ''} ${d.habit.name} 已打卡`)
      }
      changed()
    } catch (e: any) {
      alert(e.message)
    }
  }

  async function submitAmount() {
    if (!asking) return
    await api.checkin(asking.habit.id, date, amount === '' ? null : Number(amount))
    toast(`${asking.habit.icon || ''} ${asking.habit.name} 已打卡`)
    setAsking(null)
    changed()
  }

  if (days.length === 0)
    return <Empty compact>还没有打卡项。到 <a href="#/checkins">打卡</a> 页添加，如 💊 吃药、🏃 运动、😴 早睡。</Empty>
  return (
    <>
      <div className="habits">
        {days.map((d) => (
          <button key={d.habit.id} className={'habit' + (d.checkin ? ' done' : '')} onClick={() => tap(d)} aria-pressed={!!d.checkin}>
            <span className="emoji">{d.habit.icon || '•'}</span>
            <span style={{ minWidth: 0 }}>
              <div className="name ellipsis">{d.habit.name}</div>
              <div className="state">
                {d.checkin
                  ? d.checkin.value != null ? `${fmtNum(d.checkin.value)} ${d.habit.unit || ''}` : '已完成'
                  : d.habit.target != null ? `目标 ${fmtNum(d.habit.target)} ${d.habit.unit || ''}` : '未完成'}
              </div>
            </span>
            <span className="check">{d.checkin ? <CircleCheck size={22} strokeWidth={2.2} /> : <Circle size={22} strokeWidth={1.6} />}</span>
          </button>
        ))}
      </div>
      {asking && (
        <Modal title={`${asking.habit.icon || ''} ${asking.habit.name}`} onClose={() => setAsking(null)}>
          <form onSubmit={(e) => { e.preventDefault(); submitAmount() }}>
            <div className="modal-body">
              <label className="field">
                <span className="lbl">今天完成了多少？</span>
                <div className="row" style={{ flexWrap: 'nowrap' }}>
                  <input className="input num" autoFocus inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} style={{ maxWidth: 160, fontSize: 18, height: 44 }} />
                  <span className="ink2">{asking.habit.unit}</span>
                </div>
              </label>
            </div>
            <div className="modal-foot"><span className="spacer" /><button type="button" className="btn" onClick={() => setAsking(null)}>取消</button><button className="btn primary">打卡</button></div>
          </form>
        </Modal>
      )}
    </>
  )
}

export function Checkins() {
  const { entity, toast } = useApp()
  const [date, setDate] = useState(today())
  const [adding, setAdding] = useState(false)
  const { data: day } = useLoad(() => api.checkinDay(date), [date])
  const from = addDays(today(), -29)
  const { data: stats } = useLoad(() => api.checkinStats(from, today()), [])
  const { data: archived } = useLoad(() => api.list('habits', { archived: 1 }), [])
  const days = Array.from({ length: 30 }, (_, i) => addDays(from, i))
  const isToday = date === today()

  return (
    <>
      <div className="page-head">
        <div className="titles"><h1>打卡</h1><div className="sub">每天的小习惯，点一下就记上</div></div>
        <button className="btn primary" onClick={() => setAdding(true)}><Plus size={16} />新打卡项</button>
      </div>
      <div className="stack">
        <section className="card">
          <div className="card-head">
            <h2>{isToday ? '今天' : fmtDate(date)}</h2>
            <IconButton label="前一天" small onClick={() => setDate(addDays(date, -1))}><ChevronLeft size={18} /></IconButton>
            <IconButton label="后一天" small onClick={() => !isToday && setDate(addDays(date, 1))} className={isToday ? 'muted' : ''}><ChevronRight size={18} /></IconButton>
            {!isToday && <button className="btn sm ghost" onClick={() => setDate(today())}>回到今天</button>}
          </div>
          {day && <HabitGrid days={day.habits} date={day.date} />}
        </section>

        <section className="card">
          <div className="card-head"><h2>最近 30 天</h2><span className="muted small">{fmtDate(from)} – 今天</span></div>
          {stats?.length === 0 && <Empty compact>暂无</Empty>}
          <div>
            {stats?.map((s) => (
              <div key={s.habit.id} className="habit-row">
                <a href="#" onClick={(e) => { e.preventDefault(); openRecord('habits', s.habit.id) }} className="row" style={{ color: 'inherit', flexWrap: 'nowrap', minWidth: 0 }}>
                  <span style={{ fontSize: 18 }}>{s.habit.icon || '•'}</span>
                  <span className="ellipsis" style={{ fontWeight: 600 }}>{s.habit.name}</span>
                </a>
                <div className="heat" aria-label={`${s.habit.name}最近30天完成${s.days_done}天`}>
                  {days.map((d) => (
                    <span key={d} title={`${fmtDate(d)}${s.dates.includes(d) ? ' ✓' : ''}`} className={(s.dates.includes(d) ? 'on' : '') + (d === today() ? ' today' : '')} />
                  ))}
                </div>
                <div className="row" style={{ flexWrap: 'nowrap', gap: 14 }}>
                  <span className="streak" title="当前连续天数"><Flame size={14} />{s.current_streak} 天</span>
                  <span className="muted small nowrap num">{Math.round(s.completion_rate * 100)}%{s.total_value != null ? ` · ${fmtNum(s.total_value)}${s.habit.unit || ''}` : ''}</span>
                </div>
              </div>
            ))}
          </div>
        </section>
        {archived && archived.rows.length > 0 && (
          <div className="small muted row">
            已归档：{archived.rows.map((h) => (
              <a key={h.id} href="#" onClick={(e) => { e.preventDefault(); openRecord('habits', h.id) }}>{h.icon} {h.name}</a>
            ))}
          </div>
        )}
      </div>
      {adding && (
        <Modal title="新打卡项" icon={<KindIcon kind="habits" />} onClose={() => setAdding(false)}>
          <EntityForm entity={entity('habits')} onCancel={() => setAdding(false)} onSaved={() => { setAdding(false); toast('已添加') }} />
        </Modal>
      )}
    </>
  )
}
