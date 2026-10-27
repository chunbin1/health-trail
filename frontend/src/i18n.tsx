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

const STORE_KEY = 'health-trail.lang'
const OLD_STORE_KEY = 'health-vault.lang' // before the app was called Health Trail

export const getLang = () => lang
export const isEN = () => lang === 'en'

// The language saved in this browser, used until the server's is known
// and when it can't be reached.
export function storedLang(): Lang {
  try {
    return (localStorage.getItem(STORE_KEY) ?? localStorage.getItem(OLD_STORE_KEY)) === 'en' ? 'en' : 'zh'
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
  document.title = t('健康足迹')
  // the name an iPhone gives it on the home screen
  document.querySelector('meta[name="apple-mobile-web-app-title"]')?.setAttribute('content', t('健康足迹'))
  document.querySelector('link[rel="manifest"]')?.setAttribute('href', l === 'en' ? '/manifest.en.webmanifest' : '/manifest.webmanifest')
}

type Plural = readonly [one: string, other: string]

const EN = {
  // app shell
  '健康足迹': 'Health Trail',
  '今天': 'Today',
  'AI助手': 'AI assistant',
  '打卡': 'Check-ins',
  '打卡|button': 'Check off',
  '记录': 'Records',
  '数据': 'Data',
  '健康问题': 'Health issues',
  '症状日志': 'Symptom log',
  '就诊': 'Visits',
  '用药': 'Medications',
  '检查报告': 'Reports',
  '化验单': 'Lab report',
  '选择首页显示的指标': 'Choose metrics for Today',
  '首页没有选中的指标。': 'No metrics chosen for Today.',
  '调整首页': 'Arrange Today',
  '数字概览': 'Numbers at a glance',
  '首页没有选中的模块。': 'No cards chosen for Today.',
  '选择模块': 'Choose cards',
  '拖动左边的把手调整顺序；窗口够宽时，窄的模块按顺序两个一行。关掉的模块只是不在首页显示；没有内容的模块（比如没有待复诊）也不显示。': 'Drag the handle on the left to reorder; in a wide enough window, narrow cards sit two to a row in this order. Cards turned off only leave Today, and cards with nothing in them (such as no follow-ups) aren’t shown either.',
  '选择指标': 'Choose metrics',
  '完成': 'Done',
  '身体数据': 'Body metrics',
  '连接 AI': 'Connect AI',
  '更多': 'More',
  '搜索': 'Search',
  '语言': 'Language',
  '设置': 'Settings',
  '正在切换…': 'Switching…',
  'Claude 聊天': 'Claude chat',
  '其他 AI': 'Other AI',
  '你用的是哪个 AI？': 'Which AI do you use?',
  'Claude 桌面 App 里的普通对话。在 Claude 的「设置 → 开发者 → 编辑配置」里，把下面的内容合并进 claude_desktop_config.json，然后用 ⌘Q 完全退出 Claude 再重新打开。':
    'Ordinary chats in the Claude desktop app. In Claude, open Settings → Developer → Edit Config, merge the following into claude_desktop_config.json, then quit Claude completely with ⌘Q and open it again.',
  '终端里的 claude 命令和 Claude 桌面 App 的「Code」页用的是同一份设置。在终端运行一次，对所有项目生效。':
    'The claude command in a terminal and the Code tab of the Claude desktop app share one setting. Run this once in a terminal; it applies to all projects.',
  '时间': 'Time',
  '现在': 'Now',
  '允许读取的文件夹': 'Folders Claude may read',
  'App Store 版受系统沙盒限制，Claude 只能把「下载」里的文件存成附件。在这里添加别的文件夹（比如桌面、文稿），Claude 就能存那里的文件。添加或移除后，已经连着的 Claude 要重新连接一次。': 'The App Store version runs in the system sandbox, so Claude can only attach files from Downloads. Add other folders here (such as Desktop or Documents) and Claude can attach files from them too. After adding or removing one, reconnect Claude if it is already connected.',
  '不再允许读取这个文件夹？\n\n{path}\n\nClaude 之后就存不了这里的文件；已经连着的 Claude 要重新连接一次。': 'Stop allowing this folder?\n\n{path}\n\nClaude will no longer be able to attach files from it. Reconnect Claude if it is already connected.',
  '等待选择…': 'Waiting for your choice…',
  '添加…': 'Add…',
  '移除中…': 'Removing…',
  '移除': 'Remove',
  '备份': 'Backups',
  '把数据库和附件打包成一个 zip 文件，存到你选的文件夹': 'The database and attachments, packed into one zip file in a folder you choose',
  '备份位置': 'Backup location',
  '选择或填写一个文件夹，如 /Users/你/Documents/HealthTrailBackups': 'Choose or type a folder, e.g. /Users/you/Documents/HealthTrailBackups',
  '选择…': 'Choose…',
  '用 iCloud 云盘': 'Use iCloud Drive',
  '建议放在 iCloud 云盘、移动硬盘或网盘的同步文件夹里，别和数据放在同一块硬盘上。': 'Best kept in iCloud Drive, on an external drive or in a synced cloud folder, not on the same disk as the data.',
  '自动备份': 'Automatic backups',
  '每天': 'Every day',
  '每周': 'Every week',
  '周日': 'Sunday',
  '周一': 'Monday',
  '周二': 'Tuesday',
  '周三': 'Wednesday',
  '周四': 'Thursday',
  '周五': 'Friday',
  '周六': 'Saturday',
  '有改动后': 'After changes',
  '记录或修改后，等 10 分钟没有新改动就备份一次': 'Once nothing has changed for 10 minutes after an edit',
  '关闭窗口或退出 App 时': 'When the window closes or the app quits',
  '开机自动启动': 'Open at login',
  '软件更新': 'Software update',
  '当前版本 {version}': 'Current version {version}',
  '检查更新': 'Check for updates',
  '正在检查…': 'Checking…',
  '已是最新版本 · 上次检查 {time}': 'Up to date · checked {time}',
  '新版本 {version}': 'Version {version}',
  '发布于 {date}': 'Released {date}',
  '更新并重启': 'Update and restart',
  '打开下载页面': 'Open the download page',
  '发布页面': 'Release page',
  '正在备份…': 'Backing up…',
  '正在下载 {pct}%': 'Downloading {pct}%',
  '正在下载…': 'Downloading…',
  '正在重启…': 'Restarting…',
  '更新到最新版本？\n\n设置了备份的话，会先备份一次数据；然后下载最新的版本、检查签名，替换后重新打开健康足迹。': 'Update to the latest version?\n\nIf backups are set up, your data is backed up first; then the latest version is downloaded, its signature checked, and Health Trail reopens.',
  '自动检查更新': 'Check for updates automatically',
  '每隔几个小时、窗口回到前台时向 GitHub 查有没有新版本，只问版本号，不会发送任何健康数据。': 'Asks GitHub every few hours, and when the window comes to the front, whether there’s a new version; only the version is asked for, never any health data.',
  '设置 · 有新版本': 'Settings · update available',
  '有新版本': 'Update available',
  '更新说明': 'What’s new',
  'Mac 没有允许健康足迹发通知，吃药、预约和打卡提醒都不会弹出。': 'Your Mac doesn’t allow Health Trail’s notifications, so no medication, appointment or check-in reminder will show.',
  '打开通知设置': 'Open Notification Settings',
  '同步到 iPhone': 'Sync to iPhone',
  '写进 Mac 的「日历」和「提醒事项」里叫「{title}」的日历和列表，iCloud 会同步到 iPhone 和手表，在那边也会提醒。': 'Writes to a calendar and a list named “{title}” in the Mac’s Calendar and Reminders; iCloud brings them to your iPhone and watch, which remind you too.',
  '预约写进「日历」': 'Appointments to Calendar',
  '前一天晚上 8 点和开始前提醒': 'Reminds at 8 pm the evening before and before it starts',
  '吃药写进「提醒事项」': 'Medications to Reminders',
  '在 iPhone 上勾掉，这里就记一次吃药；这里记了，那边也会勾掉': 'Tick one off on your iPhone and a dose is logged here; log it here and it’s ticked off there',
  '请在系统弹窗里点「允许」…': 'Click “Allow” in the system prompt…',
  '打开隐私设置': 'Open Privacy Settings',
  '允许访问': 'Allow access',
  '上次同步 {time}': 'Last synced {time}',
  '登录 Mac 后在菜单栏运行，不打开窗口，到点照常提醒。关掉窗口也会留在菜单栏；要完全退出，点菜单栏的图标 →「退出健康足迹」。': 'Runs in the menu bar after you log in to your Mac, without opening the window, and reminds you on time. Closing the window also leaves it in the menu bar; to quit completely, click its menu bar icon → “Quit Health Trail”.',
  '最多保留': 'Keep up to',
  '份，更早的自动删除': 'backups; older ones are deleted',
  'App 开着的时候才会自动备份；错过的时间会在下次打开时补上。': 'Automatic backups run while the app is open; a missed one is made the next time it opens.',
  '备份记录': 'Backups made',
  '备份中…': 'Backing up…',
  '立即备份': 'Back up now',
  '这个文件夹里还没有备份': 'No backups in this folder yet',
  '先在上面选好备份位置': 'Choose a backup location above first',
  '恢复前': 'Before restore',
  '更新前': 'Before update',
  '恢复中…': 'Restoring…',
  '恢复': 'Restore',
  '已备份：{name}': 'Backed up: {name}',
  '恢复到 {time} 的备份？\n\n恢复前会先把当前数据另存一份（文件名以 before-restore 结尾），恢复错了可以从那份再恢复回来。': 'Restore the backup from {time}?\n\nYour current data is saved first (in a file ending in before-restore), so you can restore that if this was a mistake.',
  '已恢复到 {time} 的数据': 'Restored the data from {time}',
  '上次备份失败（{time}）：{error}': 'The last backup failed ({time}): {error}',
  '上次备份：{time} · {file}': 'Last backup: {time} · {file}',
  '搜索所有记录，如 胃、头孢、协和': 'Search all records, e.g. stomach, cefalexin, a hospital',
  '没有找到「{q}」': 'Nothing found for “{q}”',
  '无法连接到健康足迹服务：{error}': 'Can’t reach the Health Trail service: {error}',
  '服务器返回了无法解析的内容（{status}）': 'The server sent a response that couldn’t be read ({status})',
  '请求失败（{status}）': 'Request failed ({status})',

  // shared
  '取消': 'Cancel',
  '保存': 'Save',
  '保存中…': 'Saving…',
  '关闭': 'Close',
  '编辑': 'Edit',
  '删除': 'Delete',
  '确定': 'OK',
  '好': 'OK',
  '请确认': 'Please confirm',
  '提示': 'Notice',
  '全部': 'All',
  '暂无': 'None yet',
  '加载中…': 'Loading…',
  '已添加': 'Added',
  '已保存': 'Saved',
  '查看': 'View',
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
  '归档': 'Archive',
  '已归档': 'Archived',
  '全部打卡项': 'All check-in items',
  '在用': 'In use',
  '共打卡 {n} 天': 'Days checked off: {n}',
  '最近一次 {date}': 'last {date}',
  '还没打过卡': 'Not checked off yet',
  '没有在用的打卡项': 'No check-in items in use',
  '还没有归档的打卡项': 'No archived check-in items yet',
  '不再打卡的打卡项可以归档：打卡页上就不显示了，打过的卡都留在这里；恢复后又会回到打卡页。': 'Archive an item you no longer check off: it leaves the Check-ins page, and its check-ins stay here. Restore it to bring it back.',
  '已归档「{name}」：以后不用再打卡，打过的记录都还在': 'Archived “{name}”: no more check-ins needed, and the ones done are kept',
  '已恢复「{name}」': 'Restored “{name}”',
  '{name}：{error}': '{name}: {error}',
  '删除这个附件？': 'Delete this attachment?',
  '删除附件': 'Delete attachment',
  '这种文件不能直接打开，已在访达中显示': 'This kind of file can’t be opened here, so it’s shown in the Finder',
  '参考 {range}': 'Ref. {range}',
  '添加一项': 'Add a result',
  '附件': 'Attachments',
  '上传': 'Upload',
  '报告原件、处方照片等可以放在这里': 'Keep the original report, prescription photos and the like here',
  '创建于 {created} · 更新于 {updated}': 'Created {created} · Updated {updated}',
  '已记录：{name} {amount} · {time}': 'Logged: {name}, {amount} · {time}',
  '吃了': 'Took it',
  '共 {n} 颗': ['{n} pill in all', '{n} pills in all'],
  '还没有指标。把报告截图发给 AI 录入，或手动添加。': 'No results yet. Send your AI a screenshot of the report, or add them by hand.',
  '服用 {name} {amount}': 'Took {name}, {amount}',

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
  '只能给今天打卡，过去的记录只能查看': 'Only today can be checked off; past days are read-only',

  // connect
  '复制': 'Copy',
  '已复制': 'Copied',
  '复制失败，请手动选择': 'Couldn’t copy; select the text by hand',
  '让 Claude、Codex 等 AI 读写你的健康档案，帮你录入和分析': 'Let Claude, Codex or another AI read and write your health records, to log and analyze them for you',
  '两种方式都可以：用 API 接窗口里的 AI 助手，或用 MCP 让 Claude、Codex 等读写档案。': 'Two ways: connect the assistant in this window with an API, or use MCP so Claude, Codex and others can read and write your records.',
  '方式一 · API': 'Way 1 · API',
  '给窗口里的 AI 助手用。填一个兼容 OpenAI 的接口：地址、密钥和模型。密钥只存在这台电脑上。声音用 Mac 本机识别，不经过这个接口；图片会发给它。': 'For the AI assistant in this window. Fill in an OpenAI-compatible service: address, key and model. The key stays on this computer. Speech is recognized on the Mac and is not sent to this service; pictures are.',
  '方式二 · MCP': 'Way 2 · MCP',
  '让 Claude、Codex 等外部 AI 通过 MCP 读写这台电脑上的档案。选你用的那个，把配置抄过去。': 'Lets an outside AI such as Claude or Codex read and write the records on this computer through MCP. Pick the one you use and copy its configuration.',
  '接好以后可以直接对 AI 说：「把这张化验单录进去」（附上截图）、「今天吃过药了」、「我近两年的尿酸怎么变化的」。':
    'Once connected, just tell your AI: “Add this lab report” (with a screenshot), “I took my meds today”, “How has my uric acid changed over the last two years?”',
  'Claude 聊天输入框的 {plus} 菜单里还有分析模板：解读最近的化验单、整理就诊摘要、分析症状、健康状况总评、打卡复盘。':
    'In Claude chat, the {plus} menu in the message box also has analysis templates: reading recent lab results, a visit summary, symptom analysis, an overall health review and a check-in review.',
  'AI 读取记录时，相关内容会发送给它的服务商处理（Claude 发给 Anthropic，Codex 发给 OpenAI）；数据本身只存在这台电脑上。AI 的分析不能代替医生诊断。':
    'When an AI reads your records, what it reads is sent to its provider for processing (Anthropic for Claude, OpenAI for Codex); the data itself stays on this computer. No AI’s analysis is a substitute for a doctor’s diagnosis.',
  '在终端运行一次；Codex 命令行和编辑器插件共用这个设置。': 'Run once in a terminal; the Codex CLI and editor extension share this setting.',
  '或者手动加进 ~/.codex/config.toml：': 'Or add it to ~/.codex/config.toml by hand:',
  '其他支持 MCP 的 AI': 'Other AI apps that support MCP',
  'Cursor、Cherry Studio 等：添加一个 MCP 服务器，类型选「命令行（stdio）」，填下面的命令和参数；或者类型选 HTTP，填地址（需要本 App 开着）。':
    'Cursor, Cherry Studio and others: add an MCP server, choose the command-line (stdio) type and fill in the command and arguments below; or choose HTTP and use the address (this app must be open).',
  '命令': 'Command',
  '参数': 'Arguments',
  'HTTP 地址': 'HTTP address',
  '在终端运行一次，对所有项目生效。': 'Run once in a terminal; it applies to all projects.',
  '或者在本 App 运行时，通过 HTTP 连接：': 'Or, while this app is running, connect over HTTP:',
  '手机访问': 'Phone access',
  '同一 Wi-Fi 或 Tailscale 网络下的手机，用浏览器打开链接即可记录和打卡；在 Safari 里「添加到主屏幕」后就像一个 App。':
    'A phone on the same Wi-Fi or Tailscale network can open the link in a browser to log records and check in. Added to the Home Screen from Safari, it works like an app.',
  '允许手机访问': 'Allow phone access',
  '本地服务没有启动：{error}': 'The local service isn’t running: {error}',
  '链接里带着访问令牌，只发给自己，不要分享给别人。': 'The link carries an access token: send it only to yourself and never share it.',
  '没有找到局域网地址，请检查网络连接。': 'No local network address found; check the network connection.',
  '手机上的 AI App（如 Claude、ChatGPT）只能连接公网服务器，这部分放在第二期（需要隧道和登录授权）。':
    'AI apps on a phone (such as Claude or ChatGPT) can only connect to servers on the internet; that is planned for phase two (it needs a tunnel and sign-in).',
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
  '新指标': 'New metric',
  '＋ 新指标…': '+ New metric…',
  '新指标的名称': 'Name of the new metric',
  '名称，如：尿酸': 'Name, e.g. Uric acid',
  '以后记这个指标会自动带上单位，也可以在身体数据页修改': 'Its unit is filled in from now on; you can change it on the Body metrics page',
  '修改这个指标': 'Edit this metric',
  '舒张压': 'Diastolic',
  '记一笔': 'Log a reading',
  '选择直接显示的指标': 'Choose the metrics to show',
  '拖动左边的把手调整顺序。关掉的指标只是不在这一页列出，记录都还在，随时可以在这里打开。': 'Drag the handle on the left to reorder. Metrics turned off only leave this page\'s list; their readings stay, and you can turn them back on here.',
  '拖动左边的把手调整顺序。关掉的指标只是不在首页显示，记录都还在「身体数据」里。': 'Drag the handle on the left to reorder. Metrics turned off only leave Today; their readings stay in Body metrics.',
  '拖动调整顺序': 'Drag to reorder',
  '拖动调整「{name}」的顺序': 'Drag to move {name}',
  '自动计算': 'Worked out for you',
  'BMI = 体重（kg）÷ 身高（m）的平方。每次记体重，都按那时的身高算一次，不用单独记。': 'BMI = weight (kg) ÷ height (m) squared. Each weight you log is worked out with your height at the time, so there is nothing to log here.',
  '中国成人标准：低于 18.5 偏瘦，18.5–23.9 正常，24–27.9 超重，28 及以上肥胖。': 'Chinese adult standard: under 18.5 underweight, 18.5–23.9 normal, 24–27.9 overweight, 28 and over obese.',
  '身高 {n}': 'Height {n}',
  '先记一次身高': 'Log your height first',
  '记体重': 'Log weight',
  '记一次身高，就能按体重算出 BMI': 'Log your height once, and BMI is worked out from your weight',
  '数值': 'Value',
  '情境': 'Context',
  '如 空腹、晨起': 'e.g. fasting, on waking',
  '时间（不填为现在）': 'Time (blank for now)',
  '这段时间没有{type}的记录': 'No {type} readings in this period',

  // overview
  '症状|quick': 'Symptom',
  '就诊|quick': 'Visit',
  '用药|quick': 'Medication',
  '预约|quick': 'Appointment',
  '{time} 吃的': 'Taken at {time}',
  '吃了哪一顿，就点哪个时间': 'Click the time of the dose you took',
  '记 {time} 这一次吃药': 'Log the {time} dose',
  '已记录：{name} {time} 这一次': 'Logged: {name}, the {time} dose',
  '接下来的预约': 'Coming up',
  '没有待去的预约。跟 AI 说「下周三下午 3 点做胃镜」就能记下，到时间前会提醒': 'No appointments coming up. Tell your AI “gastroscopy next Wednesday at 3 pm” to add one; a reminder comes before it.',
  '明天': 'Tomorrow',
  '已过': 'Past',

  // appointments
  '预约': 'Appointments',
  '约好的检查、复诊和体检，到时间前会提醒': 'Exams, follow-ups and checkups you’ve booked, with a reminder before each',
  '接下来': 'Coming up',
  '以往': 'Earlier',
  '去过之后': 'After the appointment',
  '把结果记下来，这条预约会标记为已完成': 'Record how it went, and this appointment is marked done',
  '记为就诊': 'Record as a visit',
  '记为检查报告': 'Record as a report',
  '取消预约': 'Cancel appointment',
  '已记录，预约标记为已完成': 'Recorded; the appointment is marked done',
  '预约已取消，不会再提醒': 'Appointment cancelled; no more reminders',
  '删除这个时间': 'Remove this time',
  '添加时间': 'Add a time',
  '身体数据|quick': 'Body metric',
  '健康问题|quick': 'Health issue',
  '夜深了': 'It’s late',
  '早上好': 'Good morning',
  '中午好': 'Good afternoon',
  '下午好': 'Good afternoon',
  '晚上好': 'Good evening',
  '这里会汇总你的健康状况。可以用 {capture} 说话或打字记下，也可以到 {link} 接好外部 AI，把体检报告截图发给它。':
    'This page sums up your health. Use {capture} to speak or type a note, or {link} and send your AI a screenshot of a checkup report.',
  '说一句或打一段字，整理成记录，你确认以后才写入。': 'Say or type something. It becomes records you confirm before anything is saved.',
  '像聊天一样说，也可以问档案里的报告和症状，或发化验单、处方的照片。要记下的内容，自动保存开着时会写入档案。': 'Chat as you would with an assistant. Ask about the reports and symptoms already on file, or send a photo of a lab report or prescription. What you want recorded is written into your records when automatic saving is on.',
  '像聊天一样说，也可以问档案里的报告和症状，或发化验单、处方的照片。要记下的内容，自动保存开着时会写入档案。用药建议不会自动写入，要点保存。': 'Chat as you would with an assistant. Ask about the reports and symptoms already on file, or send a photo of a lab report or prescription. What you want recorded is written into your records when automatic saving is on. A suggested medicine is not written until you save it.',
  '声音只在这台 Mac 上识别，不会上传。整理时，这段文字，以及已有健康问题、用药和打卡项的名称，会发给你填写的接口。整理出来的内容不能代替医生的诊断。': 'Speech is recognized on this Mac and is not uploaded. When you organize a note, that text — and the names of existing health issues, medications and check-ins — is sent to the service you configured. What comes back is not a diagnosis.',
  '声音只在这台 Mac 上识别，不会上传。文字和图片会发给你填写的接口，同时带上已有健康问题、用药和打卡项的名称。整理出来的内容不能代替医生的诊断。': 'Speech is recognized on this Mac and is not uploaded. Text and pictures are sent to the service you configured, along with the names of existing health issues, medications and check-ins. What comes back is not a diagnosis.',
  '接口地址': 'Service address',
  '密钥': 'API key',
  '模型': 'Model',
  '比如 https://api.openai.com/v1': 'e.g. https://api.openai.com/v1',
  '比如 gpt-4o-mini': 'e.g. gpt-4o-mini',
  '已保存密钥，留空表示不改': 'A key is saved. Leave this blank to keep it.',
  '保存接口': 'Save service',
  '正在保存接口…': 'Saving service…',
  '接口已保存': 'Service saved',
  '忘掉密钥': 'Forget the key',
  '还没有填写接口。填好以后就可以在这里说话或打字记档案。': 'No service yet. Once you fill this in, you can speak or type to record here.',
  '修改接口': 'Change service',
  '把你说的话或打的字整理成档案里的记录。密钥只存在这台电脑上，不放进备份。声音用 Mac 本机识别，不经过这个接口。': 'Turns what you say or type into records. The key stays on this computer and is not included in backups. Speech is recognized on the Mac and is not sent to this service.',
  '把你说的话、打的字或发的图片整理成档案里的记录。密钥只存在这台电脑上，不放进备份。声音用 Mac 本机识别，不经过这个接口；图片会发给它。': 'Turns what you say, type or send as a picture into records. The key stays on this computer and is not included in backups. Speech is recognized on the Mac and is not sent to this service; pictures are.',
  '说或输入要记的事，例如：今天早上头痛，大概 4 分': 'Say or type what to record, e.g. headache this morning, about a 4',
  '输入消息，或发一张化验单。Enter 发送': 'Type a message, or send a lab photo. Enter to send',
  '发送': 'Send',
  '添加图片': 'Add a picture',
  '新对话': 'New chat',
  '历史': 'History',
  '返回': 'Back',
  '还没有对话记录': 'No chats yet',
  '{n} token': '{n} tokens',
  '输入 {prompt} · 输出 {completion} · 合计 {total} · 缓存 {cached}': '{prompt} in · {completion} out · {total} total · {cached} cached',
  '只能发送图片（JPG、PNG、GIF、WebP）': 'Only pictures can be sent (JPG, PNG, GIF, WebP)',
  '这张图片读不出来': 'Couldn’t read this picture',
  '这张图片太大': 'This picture is too large',
  '一次最多 4 张图片': 'Up to 4 pictures at a time',
  '（图片）': '(picture)',
  '正在思考…': 'Thinking…',
  '自动保存': 'Save automatically',
  '手动保存': 'Save manually',
  '模式': 'Mode',
  '推荐': 'Recommended',
  '回复一到就写入档案': 'A reply is written as soon as it arrives',
  '回复一到就写入档案，用药要另外点保存': 'A reply is written as soon as it arrives. Medicines wait for you to save them.',
  '看过、改过之后再保存': 'Look it over and edit it, then save',
  '回复一到就写入档案，不用再点保存': 'A reply is written into your records as soon as it arrives',
  '记录已保存，但图片没能存成附件：{error}': 'The record was saved, but the picture could not be attached: {error}',
  '这张图片已经不在这次对话里，请重新发一次再保存。': 'That picture is no longer in this chat. Send it again, then save.',
  '开始听': 'Start listening',
  '结束听写': 'Stop listening',
  '正在听…': 'Listening…',
  '整理': 'Organize',
  '正在整理…': 'Organizing…',
  '保存到档案': 'Save to records',
  '正在保存…': 'Saving…',
  '放弃': 'Discard',
  '修改': 'Edit',
  '收起': 'Hide',
  '去掉': 'Remove',
  '已记入档案': 'Saved to your records',
  '没有整理出可以保存的记录。': 'Nothing to save came out of that.',
  '请先改掉标出的问题，再保存。': 'Fix the problems marked below, then save.',
  '已经写进档案的已标出。为避免记重，这次不能再保存。还没写上的，请到对应的页面补记。': 'What was saved is marked. This preview can’t be saved again, so nothing is written twice. Add anything that didn’t save from its own page.',
  '相关健康问题的名称': 'Health issue name',
  '对应药物的名称': 'Medication name',
  '打卡项': 'Check-in',
  '数量': 'Amount',
  '备注': 'Notes',
  '标题': 'Title',
  '全部字段': 'All fields',
  '添加一条指标': 'Add a result',
  '删掉这条指标': 'Remove this result',
  '先写一点内容，或点麦克风说。': 'Write something first, or tap the microphone.',
  '整理被中断了，刚写的还在，再点一次「发送」。': 'That reply was interrupted. What you wrote is still here — tap Send again.',
  '连接 AI|link': 'connect your AI',
  '进行中的健康问题': 'Active health issues',
  '在用药物': 'Current medications',
  '异常指标': 'Abnormal results',
  '今日打卡': 'Today’s check-ins',
  '管理': 'Manage',
  '需要留意的指标': 'Results to watch',
  '趋势': 'Trends',
  '各项指标最近一次的结果都在参考范围内': 'Every test’s latest result is within its reference range',
  '还有 {n} 项': '{n} more',
  '还有 {n} 种': '{n} more',
  '其他在用的药': 'Other current medications',
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
  '还没有{label}。点右上角添加，{br}或者跟 AI 说一句就能记下。': 'No {label} yet. Add one with the button above,{br}or just tell your AI.',
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
