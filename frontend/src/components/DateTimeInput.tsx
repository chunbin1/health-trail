import DatePicker, { registerLocale } from 'react-datepicker'
import { enUS } from 'date-fns/locale/en-US'
import { zhCN } from 'date-fns/locale/zh-CN'
import 'react-datepicker/dist/react-datepicker.css'
import { getLang, t } from '../i18n'

registerLocale('zh-CN', zhCN)
registerLocale('en-US', enUS)

// A date and time, a date, or a time of day to pick. The native inputs gave
// Safari, the Mac app's WebView, a calendar only, with the time left to typing
// into the field; react-datepicker lists the times to click, and every date
// field uses it too so all the pickers look alike. Values travel as the app's
// strings: 'YYYY-MM-DD HH:MM' for a datetime, 'YYYY-MM-DD' for a date,
// 'HH:MM' for a time.
type Mode = 'datetime' | 'date' | 'time'

const pad = (n: number) => String(n).padStart(2, '0')

function parse(v: string, mode: Mode): Date | null {
  if (!v) return null
  const [date, time = '00:00'] = mode === 'time' ? ['', v] : v.split(' ')
  const [h, mi] = time.split(':').map(Number)
  const d = new Date()
  if (date) {
    const [y, mo, day] = date.split('-').map(Number)
    d.setFullYear(y, mo - 1, day)
  }
  d.setHours(h || 0, mi || 0, 0, 0)
  return isNaN(d.getTime()) ? null : d
}

function format(d: Date, mode: Mode): string {
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const ymd = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  return mode === 'time' ? hm : mode === 'date' ? ymd : `${ymd} ${hm}`
}

export function DateTimeInput({ value, onChange, mode = 'datetime', required }: {
  value: string; onChange: (v: string) => void; mode?: Mode; required?: boolean
}) {
  return (
    <DatePicker
      selected={parse(value, mode)}
      onChange={(d: Date | null) => onChange(d ? format(d, mode) : '')}
      showTimeSelect={mode !== 'date'}
      showTimeSelectOnly={mode === 'time'}
      timeIntervals={5}
      timeCaption={t('时间')}
      timeFormat="HH:mm"
      dateFormat={mode === 'time' ? 'HH:mm' : mode === 'date' ? 'yyyy-MM-dd' : 'yyyy-MM-dd HH:mm'}
      locale={getLang() === 'en' ? 'en-US' : 'zh-CN'}
      dateFormatCalendar={getLang() === 'en' ? 'LLLL yyyy' : 'yyyy年M月'}
      isClearable={!required && !!value}
      required={required}
      className="input"
      wrapperClassName={'dt-wrap' + (mode === 'time' ? ' time' : '')}
      popperClassName="dt-popper"
      showPopperArrow={false}
      // out of the form's <label> (a click inside would count as one on the
      // label) and of the dialog's scrolling body (which would clip it)
      portalId="dt-portal"
    >
      {mode !== 'time' && (
        <div className="dt-foot">
          <button type="button" className="btn sm" onClick={() => onChange(format(new Date(), mode))}>
            {mode === 'date' ? t('今天') : t('现在')}
          </button>
        </div>
      )}
    </DatePicker>
  )
}
