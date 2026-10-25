import { useState } from 'react'
import { Bell, ChevronLeft, ChevronRight, CircleCheck, Circle, Flame, Plus, Trash2 } from 'lucide-react'
import { addDays, api, fmtDate, fmtNum, today, type HabitDay, type Rec } from '../api'
import { EntityForm } from '../components/EntityForm'
import { ask, tell } from '../components/Dialog'
import { Modal } from '../components/Modal'
import { t, tr, tx } from '../i18n'
import { openRecord, useApp, useLoad } from '../state'
import { Empty, IconButton, KindIcon } from '../ui'

// The day's habits as buttons: one tap checks in (or asks for the amount,
// for a habit with a unit, or the reading, for one that logs a body metric
// such as 体重); a tap on a done habit undoes it.
export function HabitGrid({ days, date }: { days: HabitDay[]; date: string }) {
  const { changed, toast } = useApp()
  const [asking, setAsking] = useState<HabitDay | null>(null)
  const [amount, setAmount] = useState('')
  // only today's check-ins can change; another day is there to look at
  const editable = date === today()

  async function tap(d: HabitDay) {
    try {
      if (d.checkin) {
        await api.uncheck(d.habit.id, date)
      } else if (d.habit.unit) {
        // a metric's target is a goal, not today's reading
        setAmount(d.habit.target != null && !d.habit.metric_type ? String(d.habit.target) : '')
        setAsking(d)
        return
      } else {
        await api.checkin(d.habit.id, date)
        toast(t('{name} 已打卡', { name: `${d.habit.icon || ''} ${d.habit.name}` }))
      }
      changed()
    } catch (e: any) {
      tell(e.message)
    }
  }

  async function submitAmount() {
    if (!asking) return
    await api.checkin(asking.habit.id, date, amount === '' ? null : Number(amount))
    toast(t('{name} 已打卡', { name: `${asking.habit.icon || ''} ${asking.habit.name}` }))
    setAsking(null)
    changed()
  }

  if (days.length === 0)
    return <Empty compact>{tr('还没有打卡项。到 {link} 页添加，如 💊 吃药、🏃 运动、😴 早睡。', { link: <a href="#/checkins">{t('打卡')}</a> })}</Empty>
  return (
    <>
      <div className="habits">
        {days.map((d) => (
          <button key={d.habit.id} className={'habit' + (d.checkin ? ' done' : '')} onClick={() => tap(d)} aria-pressed={!!d.checkin} disabled={!editable}>
            <span className="emoji">{d.habit.icon || '•'}</span>
            <span style={{ minWidth: 0 }}>
              <div className="name ellipsis">{d.habit.name}</div>
              <div className="state">
                {d.checkin
                  ? d.checkin.value != null ? `${fmtNum(d.checkin.value)} ${tx(d.habit.unit) || ''}` : t('已完成')
                  : d.habit.target == null && !d.habit.remind_at ? t('未完成')
                  : <>
                      {d.habit.remind_at && <><Bell size={11} style={{ verticalAlign: '-1px' }} aria-label={t('提醒')} /> {d.habit.remind_at}</>}
                      {d.habit.target != null && d.habit.remind_at && ' · '}
                      {d.habit.target != null && t('目标 {target}', { target: `${fmtNum(d.habit.target)} ${tx(d.habit.unit) || ''}` })}
                    </>}
              </div>
            </span>
            <span className="check">{d.checkin ? <CircleCheck size={22} strokeWidth={2.2} /> : <Circle size={22} strokeWidth={1.6} />}</span>
          </button>
        ))}
      </div>
      {!editable && <div className="muted small" style={{ marginTop: 10 }}>{t('只能给今天打卡，过去的记录只能查看')}</div>}
      {asking && (
        <Modal title={`${asking.habit.icon || ''} ${asking.habit.name}`} onClose={() => setAsking(null)}>
          <form onSubmit={(e) => { e.preventDefault(); submitAmount() }}>
            <div className="modal-body">
              <label className="field">
                <span className="lbl">{asking.habit.metric_type ? t('今天的{metric}是多少？', { metric: tx(asking.habit.metric_type) }) : t('今天完成了多少？')}</span>
                <div className="row" style={{ flexWrap: 'nowrap' }}>
                  <input className="input num" autoFocus inputMode="decimal" required={!!asking.habit.metric_type} value={amount} onChange={(e) => setAmount(e.target.value)} style={{ maxWidth: 160, fontSize: 18, height: 44 }} />
                  <span className="ink2">{tx(asking.habit.unit)}</span>
                </div>
              </label>
            </div>
            <div className="modal-foot"><span className="spacer" /><button type="button" className="btn" onClick={() => setAsking(null)}>{t('取消')}</button><button className="btn primary">{asking.habit.metric_type ? t('记录|button') : t('打卡|button')}</button></div>
          </form>
        </Modal>
      )}
    </>
  )
}

export function Checkins() {
  const { entity, toast, changed } = useApp()
  const [date, setDate] = useState(today())
  const [adding, setAdding] = useState(false)
  const { data: day } = useLoad(() => api.checkinDay(date), [date])
  const from = addDays(today(), -29)
  const { data: stats } = useLoad(() => api.checkinStats(from, today()), [])
  const { data: archived } = useLoad(() => api.list('habits', { archived: 1 }), [])
  const days = Array.from({ length: 30 }, (_, i) => addDays(from, i))
  const isToday = date === today()

  // a habit and all its check-ins; archiving (in its details) keeps them
  async function removeHabit(h: Rec) {
    const name = `${h.icon || ''} ${h.name}`.trim()
    const msg = t('删除打卡项「{name}」？它的所有打卡记录也会一起删除，此操作不可撤销。', { name })
      + (h.metric_type ? t('已经记进身体数据的数值会保留。') : '')
      + '\n\n' + t('只是不想再打卡的话，可以点它的名字，在详情里改成「已归档」，记录会保留。')
    if (!(await ask(msg, { ok: t('删除'), danger: true }))) return
    try {
      await api.remove('habits', h.id)
      changed()
      toast(t('已删除'))
    } catch (e: any) {
      tell(e.message)
    }
  }

  return (
    <>
      <div className="page-head">
        <div className="titles"><h1>{t('打卡')}</h1><div className="sub">{t('每天的小习惯，点一下就记上')}</div></div>
        <button className="btn primary" onClick={() => setAdding(true)}><Plus size={16} />{t('新打卡项')}</button>
      </div>
      <div className="stack">
        <section className="card">
          <div className="card-head">
            {/* 回到今天 goes left of the arrows, never right: the arrows stay at
                the edge, so its appearing doesn't move them under the finger */}
            <h2>{isToday ? t('今天') : fmtDate(date)}</h2>
            {!isToday && <button className="btn sm ghost" onClick={() => setDate(today())}>{t('回到今天')}</button>}
            <IconButton label={t('前一天')} small onClick={() => setDate(addDays(date, -1))}><ChevronLeft size={18} /></IconButton>
            <IconButton label={t('后一天')} small onClick={() => !isToday && setDate(addDays(date, 1))} className={isToday ? 'muted' : ''}><ChevronRight size={18} /></IconButton>
          </div>
          {day && <HabitGrid days={day.habits} date={day.date} />}
        </section>

        <section className="card">
          <div className="card-head"><h2>{t('最近 30 天')}</h2><span className="muted small">{t('{from} – 今天', { from: fmtDate(from) })}</span></div>
          {stats?.length === 0 && <Empty compact>{t('暂无')}</Empty>}
          <div>
            {stats?.map((s) => (
              <div key={s.habit.id} className="habit-row">
                <a href="#" onClick={(e) => { e.preventDefault(); openRecord('habits', s.habit.id) }} className="row" style={{ color: 'inherit', flexWrap: 'nowrap', minWidth: 0 }}>
                  <span style={{ fontSize: 18 }}>{s.habit.icon || '•'}</span>
                  <span className="ellipsis" style={{ fontWeight: 600 }}>{s.habit.name}</span>
                </a>
                <div className="heat" aria-label={t('{name}最近30天完成{n}天', { name: s.habit.name, n: s.days_done })}>
                  {days.map((d) => (
                    <span key={d} title={`${fmtDate(d)}${s.dates.includes(d) ? ' ✓' : ''}`} className={(s.dates.includes(d) ? 'on' : '') + (d === today() ? ' today' : '')} />
                  ))}
                </div>
                <div className="row" style={{ flexWrap: 'nowrap', gap: 14 }}>
                  <span className="streak" title={t('当前连续天数')}><Flame size={14} />{t('{n} 天', { n: s.current_streak })}</span>
                  <span className="muted small nowrap num">{Math.round(s.completion_rate * 100)}%{s.total_value != null ? ` · ${fmtNum(s.total_value)}${tx(s.habit.unit) || ''}` : ''}</span>
                  <IconButton label={t('删除打卡项')} small className="danger push-end" onClick={() => removeHabit(s.habit)}><Trash2 size={15} /></IconButton>
                </div>
              </div>
            ))}
          </div>
        </section>
        {archived && archived.rows.length > 0 && (
          <div className="small muted row">
            {t('已归档：')}{archived.rows.map((h) => (
              <a key={h.id} href="#" onClick={(e) => { e.preventDefault(); openRecord('habits', h.id) }}>{h.icon} {h.name}</a>
            ))}
          </div>
        )}
      </div>
      {adding && (
        <Modal title={t('新打卡项')} icon={<KindIcon kind="habits" />} onClose={() => setAdding(false)}>
          <EntityForm entity={entity('habits')} onCancel={() => setAdding(false)} onSaved={() => { setAdding(false); toast(t('已添加')) }} />
        </Modal>
      )}
    </>
  )
}
