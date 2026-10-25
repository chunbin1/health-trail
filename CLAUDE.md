# 给编码助手的说明

改代码前先读 [LESSONS.md](LESSONS.md)（错题本）。改完之后，如果这次有返工、误判或验证太薄的地方，往错题本里加一条。

## 这是什么

单人的本地健康档案：健康问题、症状、就诊、用药、检查报告和检验指标、身体数据、打卡。数据存在本机的 SQLite 里，Claude 通过 MCP 读写这些数据。第一期 App 里**不嵌** Claude API。

## 结构

```
main.go                  只读命令行：app（Wails 窗口 + 本地服务）/ serve / mcp（stdio）/ info，交给 internal/desktop
internal/desktop/        桌面 App 这一层
  app.go                 app 结构体、网络端口的 handler()、窗口专用的 windowHandler()、RunApp/RunServe/RunMCP/RunInfo
  listener.go            本地端口的监听、settings.json
  backups.go             备份的触发和接口（打包、恢复在 internal/backup）
  folders.go, attachments.go  只给窗口用的接口：允许读取的文件夹、在 Mac 上打开 / 删除附件
  shell.go, dock_darwin.go  菜单栏图标、关窗留在后台、开机自启；Dock 图标显隐和「是否开机启动」的原生代码
  links.go               healthtrail:// 链接（来自日历、提醒事项）跳到哪条记录
internal/calsync/        预约写进「日历」、吃药写进「提醒事项」（同步到 iPhone），勾掉就记吃药；eventkit_darwin.go 是 EventKit 的原生部分
internal/reminders/      打卡、用药、预约的系统通知（Wails 服务）
internal/folders/        沙盒版「允许读取的文件夹」：安全书签（bookmarks_darwin.go），窗口和 MCP 启动时都打开
internal/attachopen/     窗口里点附件：复制一份到临时目录，用 Mac 上对应的 App 打开（不认识的类型只在访达里显示）；副本在删除、超过 1 小时、启动和退出时清掉
internal/store/          数据层，唯一的数据来源
  schema.go              所有记录类型和字段的定义 ← 加字段从这里开始
  schema_en.go           schema 里的中文在英文界面显示成什么（有测试检查覆盖）
  store.go               建表/迁移、增删改查、校验
  convert.go             字段类型转换、日期规范化、检验值和参考范围解析
  queries.go             总览、趋势、打卡统计、搜索、时间线
internal/server/         REST API（/api/...）、安全层 guard.go、UI 静态文件
internal/mcpserver/      MCP 工具（mcp.go）和分析模板（prompts.go）
internal/backup/         备份成 zip、清理旧备份、恢复（一个事务里整表替换）、定时判断
frontend/                React + TS + Vite；embed.go 把构建结果（dist）嵌进 Go 程序
  src/i18n.tsx           中英文切换：界面文字的英文对照、t()/tx()
```

**schema.go 是唯一的数据来源**：数据表（自动迁移、只增不删）、API 接受的字段、MCP 工具说明里的字段列表、前端表单，全部从它生成。加一个字段只需改 schema.go（再在 schema_en.go 里补英文，`TestEnglishCoversSchema` 会提醒），其他地方会自动跟上；需要特殊展示的地方再改前端。

## 硬规则

- **安全**：所有经过网络端口的请求都要经过 `server.Guard`。只有桌面窗口进程内的资源服务器可以直接用 `Server.Handler()`。不要放宽 Guard 的规则：本机请求只认回环地址形式的 Host（防 DNS 重绑定，`*.localhost` 也不行）；其他设备必须带令牌；Origin 不是本站的一律拒绝。改 guard.go 必须同时更新 `server_test.go` 里的 `TestGuard`。
- **附件**：只有图片、PDF、纯文本可以直接在页面里打开，其他类型一律作为下载返回，并带 `Content-Security-Policy: sandbox`。不能让上传的 HTML 或 SVG 以本站身份运行。
- **stdout 归 MCP 协议**：`health-trail mcp` 模式下，任何日志只能写 stderr。
- **错误要让 Claude 看得懂**：业务错误返回 `store.ValidationError`，消息里写明哪个字段、允许哪些值，MCP 层通过 `failed()` 作为工具结果返回（IsError），而不是协议错误。
- **一份报告要么全部写入，要么一条不写**：批量写入用 `CreateMany`（事务）；`add_lab_report`（mcpserver 的 `importLabs`）失败时要删掉它新建的报告。
- **界面统一**：颜色和图标用 `ui.tsx` 的 `KIND`（每类记录一个图标 + `--c-<kind>` 颜色）和 `styles.css` 里的 token，不要在组件里写死颜色。图表系列颜色要先用 dataviz 的 `validate_palette.js` 校验浅色和深色两套。
- **界面文字中英两套**：前端写死的文字一律 `t('中文原文')`，并在 `i18n.tsx` 的 `EN` 里加英文（漏了类型检查不过）；schema 来的字段名、提示、枚举选项、单位用 `tx()` 显示。枚举存进数据库的值永远是中文，只翻译显示。语言保存在数据目录的 settings.json（`lang`）。MCP 的说明和 prompts 保持中文。
- **不编造医疗结论**：MCP 的 instructions 和 prompts 要求 Claude 只依据数据说话、提醒就医。改这些文字时保留这些约束。

## 验证

```bash
make test        # go vet + go test + 前端类型检查
make bin         # 构建 UI 和二进制
make app         # 打包 健康足迹.app
```

- 改了 store/server/mcpserver：`make test` 必须全绿。
- 改了 MCP 工具：用逐条发送的 stdio 驱动做一次端到端测试（见 LESSONS #1），不要一次性把请求全部塞进 stdin。
- 改了界面：用一个临时数据目录启动 `./health-trail serve -data <临时目录> -port 8731`，在浏览器里检查桌面宽度和 375px 手机宽度（不能横向溢出）。先确认数据加载完再判断（LESSONS #4），重新构建后强制刷新（LESSONS #5）。弹窗、抽屉、日期选择器都有动画：测量位置或截图前先 `document.getAnimations().forEach(a => a.finish())` 并稍等一下；截图里「没有出现」时，先用 `getBoundingClientRect` 和再截一张确认，再判断是 bug。需要容器宽度才能绘制的组件用 `useLayoutEffect` 在第一次绘制前量好。
- 改了桌面窗口：`./health-trail -data <临时目录> -port 8732` 启动测试窗口，先确认进程还在运行再截屏（LESSONS #10）。要测日历、提醒事项这类系统权限，改用 `open -n 健康足迹.app --stderr <日志> --args -data <临时目录> -port 8734`（LESSONS #25）。不在默认数据目录的测试窗口不发系统通知（会清掉真实 App 排好的提醒），日历和提醒事项写进「健康足迹（测试）」。
- **永远不要用默认数据目录做测试**，那里是用户真实的健康数据：`~/Library/Application Support/HealthVault`。`make dev-real` 读写的就是这个目录，只给用户自己用，编码助手不要运行它；要测 `dev.sh --real` 这条路径，就把 `HEALTH_TRAIL_DATA` 设成临时目录。
