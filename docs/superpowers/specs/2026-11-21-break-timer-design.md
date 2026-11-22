# 休息与喝水定时提醒（Break & Hydration Reminder）设计说明

## 1. 背景与目标
用户在专注工作时经常忘记喝水、久坐不起，需要一个类似番茄时钟但**侧重点在定时喝水和起身休息**的功能：
- 平时安静运行：Mac 菜单栏实时显示倒计时（Windows 托盘悬停提示状态），不打扰工作；
- 到点强提醒：弹出一个独立置顶的小巧休息浮窗，提示喝水并起身站立，支持推迟、跳过和记录喝水；
- 轻量记录：独立按天记录今日喝水杯数与休息次数，保留最近 7 天简明统计，不污染健康足迹的 SQLite 医疗健康档案与打卡表结构。

### 非目标
- 不将喝水/休息流水写入 SQLite 打卡或记录表，不新增或修改 MCP 工具。
- 不锁全屏或强制阻塞操作。
- 休息倒计时自然归零后**不自动关闭浮窗、不自动开始下一轮工作、不自动加休息次数**。
- 电脑休眠或合盖唤醒后**不补弹过期提醒**。

## 2. 方案对比与选择
- **方案 A（采用）：后台计时引擎 + 独立置顶休息浮窗 + 主窗口「休息」页**
  - 优点：保留原有菜单栏结构，平时不占用主窗口空间，到点只弹出小巧浮窗，打断感克制且跨平台清晰。
  - 代价：多维护一个独立无边框置顶浮窗的显隐和菜单栏同步。
- **方案 B（未选）：后台计时 + 唤起主窗口遮罩**
  - 缺点：每次提醒都拉起 1240×820 主窗口，冲击感过强。
- **方案 C（未选）：菜单栏改为 Popover 面板**
  - 缺点：破坏现有菜单栏「打开健康足迹 / 更新 / 开机启动 / 退出」的交互习惯，且 Windows 托盘行为差异大。

## 3. 核心状态机与规则

```
  idle ──(start)──▶ working ──(倒计时归零)──▶ break ──(倒计时归零)──▶ break_done
   ▲                   │                        │                        │
   │               (pause)                 (complete)               (complete)
   │                   ▼                        ▼                        ▼
   │                 paused                  working (+1 休息)        working (+1 休息)
   │                                            │                        │
   └──(stop / complete_idle / skip)─────────────┴────────────────────────┘
```

- **状态枚举**：`idle`（未开启）、`working`（工作中）、`break`（休息中）、`break_done`（休息完成待确认）、`paused`（已暂停）、`outside_hours`（非工作时段）。
- **休息自然归零行为**：进入 `break_done` 状态，保持休息浮窗显示、菜单栏显示「休息完成」，等待用户回到电脑前手动点击「开始下一轮」（`complete`：今日休息次数 +1，隐藏浮窗并开启下一轮工作）或「暂不开始」（`complete_idle`：今日休息次数 +1，隐藏浮窗并回到 `idle`）。休息未满提前点「休息完了」同样触发 `complete`。
- **休眠/唤醒保护**：`Tick()` 检测到相邻两次 Tick 的系统时钟跳跃超过 60 秒时，判定为系统休眠唤醒或时钟大幅跳变，**绝不触发** `OpenBreak` 或 `PlaySound`；若工作倒计时已过期，静默重置为一轮新的 `working`（若开启自动工作时段且当前不在时段内，则转为 `outside_hours`）；若休息倒计时已过期，静默隐藏浮窗并切回新的 `working` 或 `idle`/`outside_hours`。
- **工作时段自动启停（可选，默认关闭）**：开启后可设定起止时间（如 `09:00–18:30`）及「仅工作日（周一至周五）」。进入时段自动从 `outside_hours` 或未手动停止的 `idle` 进入 `working`；离开时段自动暂停/归位到 `outside_hours`。若用户在当前时段内手动停止，记录当天手动停止日期以免同一天重复自动拉起（除非次日或关闭重开自动时段）。

## 4. 数据存储与接口
- **存储位置**：数据目录下的 `breaktimer.json`（原子写入：`breaktimer.json.tmp` 重命名覆盖），与 SQLite 健康档案完全隔离；`HealthTrail-Test` 测试目录独立使用自己的 `breaktimer.json`。
- **配置默认项与校验**：
  - `work_minutes`: 45（5–180）
  - `break_minutes`: 3（1–30）
  - `snooze_minutes`: 5（1–30）
  - `water_goal`: 8（1–30）
  - `auto_schedule`: false，`schedule_start`: `"09:00"`，`schedule_end`: `"18:30"`，`weekdays_only`: true
  - `popup_enabled`: true，`sound_enabled`: true，`menu_bar_countdown`: true
- **历史保留**：磁盘保留最近 30 天每日汇总（`date`, `breaks`, `water`），接口快照返回最近 7 天（由旧到新，含今天）。
- **REST API（经 `server.Guard` 保护）**：
  - `GET /api/break-timer`
  - `PUT /api/break-timer/config`
  - `POST /api/break-timer/action`（`start` / `pause` / `resume` / `stop` / `break_now` / `complete` / `complete_idle` / `snooze` / `skip` / `water_add` / `water_sub`）
  - `POST /api/break-timer/hide`

## 5. 桌面壳与前端界面
- **菜单栏 / 托盘**：菜单顶部加入休息状态摘要（禁用项）、开始/暂停/继续、现在休息/结束休息、推迟 N 分钟、记一杯水 (+1)，下接分割线与原有菜单项；Mac 菜单栏实时显示倒计时或状态标签，Windows 托盘悬停显示状态（仅在状态、分钟桶或次数变化时刷新菜单文本，避免 Windows 菜单闪烁）。
- **独立休息浮窗**：`420×340` 无边框置顶卡片（`/?shell=...&view=break`），隐藏关闭只 Hidden 不 Destroy，支持拖拽、Esc 关闭、Web Audio 轻柔提示音，提供「喝了一杯 (+1)」「休息完了 / 开始下一轮」「推迟 N 分钟」「跳过 / 暂不开始」。
- **主窗口「休息」页**：左侧导航顶组新增「休息」（`#/breaks`），包含计时控制圆环、今日喝水与休息统计（含 7 天小柱状图）以及提醒设置表单。所有文案接入 `i18n.tsx` 中英双语，后端校验错误接入 `internal/msg`。
