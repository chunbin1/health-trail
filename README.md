# 健康足迹（Health Trail）

一个存在自己电脑上的个人健康档案：健康问题、症状、就诊、用药、体检和化验报告、身体数据、每日打卡。可以通过 MCP 接入 Claude、Codex 等 AI，让它们帮你录入和分析。

- **数据只在本机**：一个 SQLite 文件，加上一个附件目录。
- **Mac 窗口**：Wails v3（系统 WebView）+ React。
- **手机**：用手机浏览器访问的功能暂时关闭，改好后再开放。
- **AI**：14 个 MCP 工具，加 5 个分析模板，Claude、Codex 以及其他支持 MCP 的 AI 都能接。截图发给 AI 就能录入化验单，也可以直接问它「我的 CRP 怎么变化的」。

## 构建

需要 Go 1.24+、Node 20+、Xcode 命令行工具。

```bash
make app      # 生成 健康足迹.app，拖进「应用程序」即可
make bin      # 只生成命令行二进制 ./health-trail
make test     # 测试
```

开发时用 `make dev`：界面在 http://localhost:5173 ，改前端保存后马上生效；后端在 127.0.0.1:8722，改了 `.go` 文件会自动重新编译和重启。数据放在临时目录 `/tmp/health-trail-dev`，不会碰真实数据。按 Ctrl+C 全部退出。桌面窗口本身的改动还是要 `make app` 后打开 App 检查。

想用自己的真实数据边用边调试，就用 `make dev-real`：和 `make dev` 一样热更新，但读写的是真实数据目录。每次启动前会先把数据库和附件备份到 `~/Library/Application Support/HealthVault-dev-backups/<时间>`，保留最近 10 份。正式 App 开着也没关系，两边可以同时用。

## 使用

```bash
./health-trail              # 打开窗口（同时在 127.0.0.1:8721 提供本地服务）
./health-trail serve        # 不开窗口，只运行服务（比如放在常开的 Mac mini 上）
./health-trail mcp          # MCP stdio 模式，给 Claude、Codex 等 AI 用
./health-trail info         # 显示数据目录和连接 AI 的配置
```

**提醒**：给打卡项填上「提醒时间」（如 23:00），到点还没打卡，Mac 会弹系统通知。通知由桌面 App 提前排好未来 7 天，App 关着也会按时弹出；只要每周至少打开一次 App 就不会断。只有从 `健康足迹.app` 启动才有提醒，第一次会请求通知权限。

数据目录默认是 `~/Library/Application Support/HealthVault`（沿用改名前的名字），可以用 `-data` 参数或环境变量 `HEALTH_TRAIL_DATA` 修改。手动备份时复制整个目录即可。

**备份**：在 App 的「备份」页选一个文件夹（建议 iCloud 云盘或移动硬盘；有 iCloud 时可一键选用 `iCloud 云盘/HealthTrailBackups`），每次备份是一个 `health-trail-日期时间.zip`（改名前的 `health-vault-…`、`健康档案-…` 也照样列出和恢复），包含数据库和附件，默认保留最近 30 份。可以开启每天、每周、有改动后（10 分钟没有新改动时）、退出 App 时自动备份；App 开着时才会自动备份，错过的会在下次打开时补上。在备份列表里点「恢复」可以还原到那个时间点，恢复前会先自动另存一份当前数据。

## 接入 AI

App 里的「连接 AI」页面会显示带着当前程序路径的配置，可以直接复制。

**Claude 桌面版**（设置 → 开发者 → 编辑配置）：

```json
{
  "mcpServers": {
    "health-trail": { "command": "/Applications/健康足迹.app/Contents/MacOS/health-trail", "args": ["mcp"] }
  }
}
```

**Claude Code**：

```bash
claude mcp add health-trail -s user -- "/Applications/健康足迹.app/Contents/MacOS/health-trail" mcp
```

**Codex**（命令行和编辑器插件共用）：

```bash
codex mcp add health-trail -- "/Applications/健康足迹.app/Contents/MacOS/health-trail" mcp
```

**其他支持 MCP 的 AI**（Cursor、Cherry Studio 等）：添加一个 stdio 类型的 MCP 服务器，命令填上面的程序路径，参数填 `mcp`；或者用 HTTP 地址 `http://127.0.0.1:8721/mcp`（需要 App 开着）。

接好之后可以这样用：
- 「把这张化验单录进去」，附上截图；
- 「今天吃过药了」「跑了 5 公里」；
- 「我近两年的尿酸怎么变化的？」；
- 在 Claude 桌面版的「＋」菜单里选用分析模板：**解读最近的化验单**、**整理就诊摘要**、**分析症状**、**健康状况总评**、**打卡复盘**。

> AI 读取记录时，相关内容会发送给它的服务商处理（Claude 发给 Anthropic，Codex 发给 OpenAI）；数据本身只存在你的电脑上。AI 的分析不能代替医生的诊断。

## 安全

- 本地服务默认只监听 `127.0.0.1`。
- 开启「手机访问」后，其他设备必须带访问令牌（存在数据目录的 `token` 文件里）。
- 本机浏览器里的其他网站无法读取或写入数据：服务会拒绝 DNS 重绑定请求和跨站请求。
- 上传的附件中，只有图片、PDF 和纯文本可以在页面里直接打开，其他类型一律作为文件下载。

## 第二期计划

- 手机上的 AI App（Claude、ChatGPT）访问（需要公网隧道和 OAuth 登录）
- App 内直接问 Claude（Claude API）
- Apple 健康数据导入
- 数据加密、自动备份

## 许可

[MIT](LICENSE)
