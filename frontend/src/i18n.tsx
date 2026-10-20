import { Fragment, type ReactNode } from 'react'
import type { Entity, Schema } from './api'

// The UI speaks Chinese or English. Chinese is the source text: t('今天')
// shows the key itself in Chinese and EN['今天'] in English. A key that
// would mean two things in English carries a context after '|'
// ('打卡|button'), which the Chinese side drops. EN is typed, so a t() key
// without English fails the type check.
//
// Text that comes from the schema (field labels, hints, enum options,
// units) is translated with tx(), from the English the server sends with
// the schema (internal/store/schema_en.go). Enum values stay Chinese in the
// data; only what is shown changes.

export type Lang = 'zh' | 'en'

let lang: Lang = 'zh'
let schemaEN: Schema['en'] = { text: {}, nouns: {} }

const STORE_KEY = 'health-vault.lang'

export const getLang = () => lang
export const isEN = () => lang === 'en'

// The language saved in this browser, used until the server's is known
// and when it can't be reached.
export function storedLang(): Lang {
  try {
    return localStorage.getItem(STORE_KEY) === 'en' ? 'en' : 'zh'
  } catch {
    return 'zh'
  }
}

export function setLang(l: Lang, schema?: Schema) {
  lang = l
  if (schema?.en) schemaEN = schema.en
  try {
    localStorage.setItem(STORE_KEY, l)
  } catch {
    // private mode: the server's setting still keeps it
  }
  document.documentElement.lang = l === 'en' ? 'en' : 'zh-CN'
  document.title = t('健康档案')
}

type Plural = readonly [one: string, other: string]

const EN = {
  // app shell
  '健康档案': 'Health Vault',
  '今天': 'Today',
  '打卡': 'Check-ins',
  '打卡|button': 'Check off',
  '记录': 'Records',
  '数据': 'Data',
  '健康问题': 'Health issues',
  '症状日志': 'Symptom log',
  '就诊': 'Visits',
  '用药': 'Medications',
  '检查报告': 'Reports',
  '身体数据': 'Body metrics',
  '连接 Claude': 'Connect Claude',
  '更多': 'More',
  '搜索': 'Search',
  '语言': 'Language',
  '搜索所有记录，如 胃、头孢、协和': 'Search all records, e.g. stomach, cefalexin, a hospital',
  '没有找到「{q}」': 'Nothing found for “{q}”',
  '无法连接到健康档案服务：{error}': 'Can’t reach the Health Vault service: {error}',
  '服务器返回了无法解析的内容（{status}）': 'The server sent a response that couldn’t be read ({status})',
  '请求失败（{status}）': 'Request failed ({status})',

  // shared
  '取消': 'Cancel',
  '保存': 'Save',
  '保存中…': 'Saving…',
  '关闭': 'Close',
  '编辑': 'Edit',
  '删除': 'Delete',
  '全部': 'All',
  '暂无': 'None yet',
  '加载中…': 'Loading…',
  '已添加': 'Added',
  '已保存': 'Saved',
  '已删除': 'Deleted',
  '已记录': 'Logged',
  '记录|button': 'Log',
  '是': 'Yes',
  '否': 'No',
  '添加{label}': 'Add {label}',
  '编辑{label}': 'Edit {label}',
  '{n} 天': ['{n} day', '{n} days'],
  '{n} 颗': ['{n} pill', '{n} pills'],
  '项目': 'Test',
  '结果': 'Result',
  '单位': 'Unit',
  '参考范围': 'Reference range',
  '标记': 'Flag',
  '日期': 'Date',
  '报告': 'Report',
  '报告|tab': 'Reports',

  // forms
  '未选择': '— None —',
  '不关联': '— None —',

  // record drawer
  '症状记录': 'Symptoms',
  '就诊记录': 'Visits',
  '检验指标': 'Lab results',
  '打卡记录': 'Check-ins',
  '服药记录': 'Doses taken',
  '删除这条{label}？{extra}此操作不可撤销。': 'Delete this {label}? {extra}This can’t be undone.',
  '报告里的所有指标也会一起删除。': 'All results in the report are deleted with it. ',
  '{name}：{error}': '{name}: {error}',
  '删除这个附件？': 'Delete this attachment?',
  '删除附件': 'Delete attachment',
  '参考 {range}': 'Ref. {range}',
  '粘贴导入': 'Paste results',
  '添加一项': 'Add a result',
  '附件': 'Attachments',
  '上传': 'Upload',
  '报告原件、处方照片等可以放在这里': 'Keep the original report, prescription photos and the like here',
  '创建于 {created} · 更新于 {updated}': 'Created {created} · Updated {updated}',
  '已记录：{name} {amount} · {time}': 'Logged: {name}, {amount} · {time}',
  '吃了': 'Took it',
  '共 {n} 颗': ['{n} pill in all', '{n} pills in all'],
  '还没有指标。可以粘贴导入，或把报告截图发给 Claude 录入。': 'No results yet. Paste them in, or send Claude a screenshot of the report.',
  '服用 {name} {amount}': 'Took {name}, {amount}',

  // lab import
  '没有识别出指标。每行应包含：项目名、结果、单位（可省略）、参考范围（可省略）。':
    'No results recognized. Each line should have the test name, the result, and optionally the unit and reference range.',
  '已保存 {n} 项指标': ['Saved {n} result', 'Saved {n} results'],
  '导入指标到「{title}」': 'Import results into “{title}”',
  '粘贴导入化验单': 'Paste a lab report',
  '标题': 'Title',
  '报告日期': 'Report date',
  '类型': 'Type',
  '机构': 'Institution',
  '如：血常规+CRP': 'e.g. CBC + CRP',
  '医院或体检中心': 'Hospital or checkup center',
  '分组（可选）': 'Panel (optional)',
  '如：血常规、肝功能': 'e.g. CBC, liver function',
  '把化验单的表格文字粘贴到这里，例如：\n\n超敏C-反应蛋白\t111.87↑\tmg/L\t0-6\n白细胞\t17.62↑\t10^9/L\t3.5-9.5\n\n从手机复制的「一格一行」格式也可以。':
    'Paste the table text of a lab report here, e.g.:\n\nhs-CRP\t111.87↑\tmg/L\t0-6\nWBC\t17.62↑\t10^9/L\t3.5-9.5\n\nText copied on a phone, one cell per line, works too.',
  '只有截图或 PDF？直接发给 Claude，说「帮我把这份化验单录进健康档案」。':
    'Only have a screenshot or PDF? Send it to Claude and say “Add this lab report to my health vault.”',
  '识别出 {n} 项，其中 {abnormal} 项异常。请对照原报告核对，可以直接修改。':
    'Recognized {n}, {abnormal} out of range. Check them against the original report; you can edit them here.',
  '自动': 'Auto',
  '删除这一行': 'Remove this row',
  '返回修改': 'Back to the text',
  '识别': 'Recognize',
  '保存 {n} 项': ['Save {n} result', 'Save {n} results'],

  // chart
  '还没有数据': 'No data yet',
  '{n} 个读数，最近 {value} {unit}': '{n} readings, latest {value} {unit}',

  // check-ins
  '{name} 已打卡': '{name} checked off',
  '还没有打卡项。到 {link} 页添加，如 💊 吃药、🏃 运动、😴 早睡。': 'No check-in items yet. Add some on the {link} page, e.g. 💊 meds, 🏃 exercise, 😴 early night.',
  '已完成': 'Done',
  '未完成': 'Not yet',
  '提醒': 'Reminder',
  '目标 {target}': 'Goal {target}',
  '今天的{metric}是多少？': 'Today’s {metric}?',
  '今天完成了多少？': 'How much today?',
  '每天的小习惯，点一下就记上': 'Small daily habits, logged with one tap',
  '新打卡项': 'New check-in item',
  '前一天': 'Previous day',
  '后一天': 'Next day',
  '回到今天': 'Back to today',
  '最近 30 天': 'Last 30 days',
  '{from} – 今天': '{from} – today',
  '{name}最近30天完成{n}天': '{name}: done on {n} of the last 30 days',
  '当前连续天数': 'Current streak',
  '已归档：': 'Archived:',
  '只能给今天打卡，过去的记录只能查看': 'Only today can be checked off; past days are read-only',

  // connect
  '复制': 'Copy',
  '已复制': 'Copied',
  '复制失败，请手动选择': 'Couldn’t copy; select the text by hand',
  '让 Claude 读写你的健康档案，帮你录入和分析': 'Let Claude read and write your health records, to log and analyze them for you',
  '接好以后可以直接对 Claude 说：「把这张化验单录进去」（附上截图）、「今天吃过药了」、「我近两年的尿酸怎么变化的」。':
    'Once connected, just tell Claude: “Add this lab report” (with a screenshot), “I took my meds today”, “How has my uric acid changed over the last two years?”',
  '输入框的 {plus} 菜单里还有分析模板：解读最近的化验单、整理就诊摘要、分析症状、健康状况总评、打卡复盘。':
    'The {plus} menu in the message box also has analysis templates: reading recent lab results, a visit summary, symptom analysis, an overall health review and a check-in review.',
  'Claude 读取记录时，相关内容会发送给 Anthropic 处理；数据本身只存在这台电脑上。Claude 的分析不能代替医生诊断。':
    'When Claude reads your records, what it reads is sent to Anthropic for processing; the data itself stays on this computer. Claude’s analysis is no substitute for a doctor’s diagnosis.',
  'Claude 桌面版': 'Claude Desktop',
  '设置 → 开发者 → 编辑配置，把下面的内容合并进 claude_desktop_config.json，然后用 ⌘Q 完全退出 Claude 再重新打开。':
    'Settings → Developer → Edit Config: merge the following into claude_desktop_config.json, then quit Claude completely with ⌘Q and open it again.',
  '在终端运行一次，对所有项目生效。': 'Run once in a terminal; it applies to all projects.',
  '或者在本 App 运行时，通过 HTTP 连接：': 'Or, while this app is running, connect over HTTP:',
  '手机访问': 'Phone access',
  '同一 Wi-Fi 或 Tailscale 网络下的手机，用浏览器打开链接即可记录和打卡；在 Safari 里「添加到主屏幕」后就像一个 App。':
    'A phone on the same Wi-Fi or Tailscale network can open the link in a browser to log records and check in. Added to the Home Screen from Safari, it works like an app.',
  '允许手机访问': 'Allow phone access',
  '本地服务没有启动：{error}': 'The local service isn’t running: {error}',
  '链接里带着访问令牌，只发给自己，不要分享给别人。': 'The link carries an access token: send it only to yourself and never share it.',
  '没有找到局域网地址，请检查网络连接。': 'No local network address found; check the network connection.',
  '手机上的 Claude App 只能连接公网服务器，这部分放在第二期（需要隧道和登录授权）。':
    'The Claude app on a phone can only connect to servers on the internet; that is planned for phase two (it needs a tunnel and sign-in).',
  '数据库是 health.db（SQLite），附件在 files/。备份时复制整个目录即可；建议开启 FileVault 磁盘加密。':
    'The database is health.db (SQLite) and attachments are in files/. To back up, copy the whole folder. Turning on FileVault disk encryption is recommended.',
  '数据目录': 'Data folder',
  '本地服务': 'Local service',
  '未启动': 'Not running',
  '版本': 'Version',

  // metrics
  '1 年': '1 year',
  '体重、血压、血糖等日常测量': 'Everyday measurements such as weight, blood pressure and blood glucose',
  '最近': 'Latest',
  '区间变化': 'Change',
  '区间范围': 'Range',
  '{n} 次测量': ['{n} reading', '{n} readings'],
  '收缩压': 'Systolic',
  '舒张压': 'Diastolic',
  '记一笔': 'Log a reading',
  '数值': 'Value',
  '情境': 'Context',
  '如 空腹、晨起': 'e.g. fasting, on waking',
  '时间（不填为现在）': 'Time (blank for now)',
  '这段时间没有{type}的记录': 'No {type} readings in this period',

  // overview
  '症状|quick': 'Symptom',
  '就诊|quick': 'Visit',
  '用药|quick': 'Medication',
  '身体数据|quick': 'Body metric',
  '健康问题|quick': 'Health issue',
  '夜深了': 'It’s late',
  '早上好': 'Good morning',
  '中午好': 'Good afternoon',
  '下午好': 'Good afternoon',
  '晚上好': 'Good evening',
  '这里会汇总你的健康状况。先到 {link} 接好，然后把体检报告截图发给 Claude，或者说「帮我记一下：今天头痛，大概 4 分」，它会直接写进来。':
    'This page sums up your health. First {link}, then send Claude a screenshot of a checkup report, or say “Note this down: headache today, about a 4” — it writes straight in here.',
  '连接 Claude|link': 'connect Claude',
  '进行中的健康问题': 'Active health issues',
  '在用药物': 'Current medications',
  '异常指标': 'Abnormal results',
  '今日打卡': 'Today’s check-ins',
  '管理': 'Manage',
  '需要留意的指标': 'Results to watch',
  '趋势': 'Trends',
  '各项指标最近一次的结果都在参考范围内': 'Every test’s latest result is within its reference range',
  '还有 {n} 项': '{n} more',
  '没有进行中的健康问题': 'No active health issues',
  '没有在用的药物': 'No current medications',
  '上次 {date} · {total}': 'Last {date} · {total}',
  '待复诊': 'Upcoming follow-ups',
  '最近记录': 'Recent records',
  '还没有记录': 'No records yet',

  // record lists
  '诊断过的、正在处理的、长期存在的健康问题': 'Diagnosed, ongoing and long-term health issues',
  '每一次不舒服：什么时候、多严重、可能的诱因': 'Each time you felt unwell: when, how bad, possible triggers',
  '门诊、复诊、急诊、住院和线上问诊': 'Outpatient, follow-up, emergency, inpatient and online visits',
  '正在吃的、吃过的药，剂量和效果': 'Drugs you take or have taken, their dosage and effects',
  '体检、化验、影像和病理报告': 'Checkup, lab, imaging and pathology reports',
  ' · 共 {n} 条': ' · {n} in all',
  '搜索{label}': 'Search {label}',
  '没有符合条件的记录': 'No matching records',
  '还没有{label}。点右上角添加，{br}或者在 Claude 里说一句就能记下。': 'No {label} yet. Add one with the button above,{br}or just tell Claude.',
  '未填日期': 'No date',
  '程度 {n}/10': 'Severity {n}/10',

  // reports
  '指标趋势': 'Result trends',
  '每个检验项目的历次结果': 'Every result of each lab test over time',
  '还没有检验指标。导入化验单或体检报告后，{br}这里会显示每项指标的历次变化。': 'No lab results yet. Once you import a lab or checkup report,{br}each test’s history shows here.',
  '找指标，如 白细胞': 'Find a test, e.g. WBC',
  '{n} 次': ['{n} result', '{n} results'],
  '参考范围 {range}': 'Reference range {range}',
  '最近一次': 'Latest',
} satisfies Record<string, string | Plural>

export type Key = keyof typeof EN
type Vars = Record<string, string | number>

function pick(key: Key, vars?: Vars): string {
  if (lang === 'zh') return key.split('|')[0]
  const v: string | Plural = EN[key]
  if (typeof v === 'string') return v
  return Number(vars?.n) === 1 ? v[0] : v[1]
}

// t translates UI text and fills in {name} placeholders.
export function t(key: Key, vars?: Vars): string {
  const s = pick(key, vars)
  return vars ? s.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m)) : s
}

// tr is t for placeholders that are elements: a link, <b>, <br />.
export function tr(key: Key, vars: Record<string, ReactNode>): ReactNode {
  const n = typeof vars.n === 'number' || typeof vars.n === 'string' ? { n: vars.n } : undefined
  return pick(key, n).split(/(\{\w+\})/).map((part, i) => {
    const m = /^\{(\w+)\}$/.exec(part)
    return <Fragment key={i}>{m && m[1] in vars ? vars[m[1]] : part}</Fragment>
  })
}

// tx translates text that comes from the schema or is a stored enum value
// (状态「在用」, 指标「体重」, 标记「偏高」). Anything it doesn't know — what
// the user typed — is shown as it is.
export function tx(s?: string | null): string {
  if (!s) return ''
  return lang === 'zh' ? s : schemaEN.text[s] ?? s
}

// noun names one record of an entity: 「添加{noun}」.
export function noun(e: Entity): string {
  if (lang === 'zh') return e.label
  return schemaEN.nouns[e.name] ?? tx(e.label)
}

// paren appends a note in brackets: 「数值（kg）」, "Value (kg)".
export const paren = (s: string, note?: string | null) => (note ? (lang === 'zh' ? `${s}（${note}）` : `${s} (${note})`) : s)
