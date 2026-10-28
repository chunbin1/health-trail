<div align="center">

**中文** | [English](README.en.md)

<img src="frontend/public/icon.svg" width="112" alt="健康足迹">

# 健康足迹 · Health Trail

### 你的健康档案，存在你自己的电脑上。

**病历、化验单、用药、身体数据、每日打卡，一个地方记全。**<br>
拍张照、说句话，AI 帮你记好；也能接 Claude、Codex 一起分析。数据存在你自己的 Mac 上。

[![Release](https://img.shields.io/github/v/release/chunbin1/health-trail?include_prereleases&label=release&color=111111)](https://github.com/chunbin1/health-trail/releases) [![CI](https://img.shields.io/github/actions/workflow/status/chunbin1/health-trail/ci.yml?label=CI&color=111111)](https://github.com/chunbin1/health-trail/actions/workflows/ci.yml) [![License](https://img.shields.io/badge/license-MIT-111111)](LICENSE)<br>
![macOS](https://img.shields.io/badge/macOS_13+-000000?logo=apple&logoColor=white) ![MCP](https://img.shields.io/badge/MCP-15_tools-111111) ![SQLite](https://img.shields.io/badge/SQLite-local-003B57?logo=sqlite&logoColor=white)

**[下载](https://github.com/chunbin1/health-trail/releases)** &nbsp;·&nbsp; **[接入 AI](#接入-ai)** &nbsp;·&nbsp; **[从源码构建](#从源码构建)**

<br>

https://github.com/user-attachments/assets/7d57d18b-8832-4bc0-9797-f5483d23beaa

<sub>约 100 秒的介绍视频（中文旁白）。视频和截图里都是虚构的示例数据。</sub>

</div>

<br>

## 适合你吗？

化验单在相册里，病历在医院 App 里，药什么时候吃的靠记性，复诊时说不清上次的指标是多少。健康足迹把这些放进一个本地的数据库，再交给你信任的 AI 帮忙整理：

- **拍照就能录**：在 App 的 AI助手里发一张化验单照片（或者发给 Claude），它逐项录进去，偏高偏低自动标出来，原图也存成附件。
- **说一句话就记好**：「早上空腹血糖 6.4，有点头晕」，记成一条血糖、一条症状。打字或说话都行，语音在 Mac 本机识别。
- **随时问，顺手安排**：「我近两年的尿酸怎么变化的？」「血糖有点高，要做哪些检查？」AI 先查你的档案再回答，不靠猜；你说一声，复查也帮你记成预约。
- **到点提醒**：吃药、打卡、复诊，Mac 弹通知，也能同步到 iPhone 的「日历」和「提醒事项」。
- **数据只在本机**：一个 SQLite 文件加一个附件目录，备份到你自己选的地方。

适合想认真管理自己（或家人）健康记录、又不想把病历交给云服务的人。如果你只需要记步数和睡眠，iPhone 自带的「健康」就够了。

目前只支持 macOS 13 及以上，Apple 芯片和 Intel 都可以。

## 快速开始

**1 · 安装**：从 **[Releases](https://github.com/chunbin1/health-trail/releases)** 下载 `HealthTrail-<版本>.dmg`，打开后把「健康足迹」拖进「应用程序」。

<sub>dmg 用 Developer ID 签名并经过苹果公证，双击就能打开。也可以[从源码构建](#从源码构建)。以后有新版本，App 会在「设置 → 软件更新」里提示，点一下就更新。</sub>

**2 · 连接 AI**：打开 App 的「连接 AI」页。可以用 API 接窗口里的 AI 助手（填地址、密钥和模型），也可以用 MCP，按你用的 AI 复制配置（Claude 桌面版、Claude Code、Codex 或其他支持 MCP 的 AI）。

**3 · 开始用**：对 AI 说

```text
把这张化验单录进去            （附上照片或截图）
今天早上空腹血糖 6.4，有点头晕
今天早上吃过药了
帮我约 11 月 3 日早上 8 点半去医院复查血糖
帮我整理一下这次复诊要问医生的问题
```

## 能做什么

### 记录

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/overview-dark.png">
  <img src="docs/img/overview-light.png" width="900" alt="「今天」页：进行中的健康问题、在用药物、今日打卡、接下来的预约和需要留意的指标">
</picture>

| 类型 | 记什么 |
| :-- | :-- |
| **健康问题** | 正在处理的病、过敏、慢性病，症状、就诊、用药都可以挂在它下面 |
| **症状** | 什么时候、哪里、多严重 |
| **就诊** | 医院、科室、医生、诊断、医嘱 |
| **用药** | 剂量、频次、每天几点吃；点一下记「吃过了」，自动统计吃了多少颗 |
| **检查报告** | 体检、化验、影像；化验指标逐项保存，参考范围原样记录，可以看单项趋势 |
| **身体数据** | 体重、血压、血糖，或者任何你自己加的指标；和化验项目同名的指标（如尿酸），化验单上的结果也一起显示；记一次身高，每次称体重都自动算出 BMI；指标可以拖动排序，不常用的可以不在这一页列出 |
| **预约** | 复诊、检查，地点和要准备的东西（空腹、带上次的片子） |
| **打卡** | 「晨起称体重」「作息守住」，连续天数和完成率 |

报告单、处方、片子可以作为附件存在对应的记录下，在窗口里点开就看。

<table>
<tr>
<td width="50%">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/trend-dark.png">
  <img src="docs/img/trend-light.png" alt="指标趋势：尿酸五次化验的结果、参考范围和变化曲线">
</picture>
</td>
<td width="50%">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/metrics-dark.png">
  <img src="docs/img/metrics-light.png" alt="身体数据：90 天的体重曲线和区间变化">
</picture>
</td>
</tr>
</table>

### 提醒和同步到 iPhone

| | |
| :-- | :-- |
| **吃药** | 到了设定的时刻还没记吃药就提醒；提前吃过、记过就不提醒 |
| **打卡** | 到了提醒时间还没打卡就提醒 |
| **预约** | 前一天晚上 8 点和开始前（默认 1 小时）各提醒一次 |
| **iPhone** | 预约写进「日历」、吃药写进「提醒事项」，用 iCloud 同步到手机和手表；在 iPhone 上勾掉吃药提醒，App 里就记一次吃药 |

关掉窗口后 App 留在菜单栏继续运行，提醒和自动备份照常；也可以设成开机自动启动。日历里的条目带着 `healthtrail://` 链接，点一下就回到 App 里那条记录。

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/checkins-dark.png">
  <img src="docs/img/checkins-light.png" width="760" alt="打卡：今天的打卡项和最近 30 天的完成情况、连续天数">
</picture>

### 备份

选一个文件夹（建议 iCloud 云盘或移动硬盘），每次备份是一个包含数据库和附件的 zip，默认保留最近 30 份。可以每天、每周、有改动后、退出时自动备份；在列表里点「恢复」就回到那个时间点，恢复前会先把当前数据另存一份。

### 中文和英文

界面可以切换中英文；数据始终按原样保存。

## 接入 AI

有两种方式，读写的是同一份档案，可以同时用：

- **App 里的 [AI助手](#ai助手)**：接你自己填的 OpenAI 兼容接口，直接在窗口里对话、拍照、说话。
- **[通过 MCP 接入](#通过-mcp-接入)**：让 Claude、Codex 等你已经在用的 AI 读写档案。

### AI助手

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/assistant-dark.png">
  <img src="docs/img/assistant-light.png" width="900" alt="AI助手：先查档案里的血糖和症状再回答该做哪些检查，然后按你说的时间整理成一张预约卡片">
</picture>

侧栏的「AI助手」直接在 App 里和 AI 对话（今天页也有入口；填好接口后，窗口右下角的悬浮球在任何一页都能打开同一段对话）。它接你自己填的接口，用和 MCP 同一套工具按需查档案。

**能做什么**

- **问档案里的任何记录**：比如某项化验这些年的全部结果，它先查再回答。
- **打字、说话或发照片记录**：化验单、处方的照片，或者一句「早上空腹血糖 6.4，有点头晕」，整理成化验单、身体数据、症状、用药、打卡等记录。
- **改已有的记录**：比如给一个药加上提醒时间，它整理成一张「修改」卡片，列出改之前和改之后。删除记录、撤销打卡还是在记录上自己操作。
- **安排检查**：建议的检查和复查先写在回答里，写明要医生确认；你让它直接安排时间时，才整理成预约卡片，备注里写明是助手建议的时间、还没在医院挂号。
- **问病情**：根据档案里的症状、化验和报告给出诊断，并推荐可以考虑的药；每次都写明结果仅供参考，需要去医院由医生确认。

**哪些会直接写入**

| 卡片 | 怎么保存 |
| :-- | :-- |
| 新的记录：症状、身体数据、化验单、打卡、预约等 | 自动保存默认开着，回复一到就写入；输入框左边可以换成手动保存。有问题的条目不会自动写入，你改完再保存 |
| 它判断的健康问题（状态「观察中」，备注写明依据和需医生确认） | 不自动保存，你看过、点一下才写入；只在你问自己可能是什么病时才有，已有同名的不再写 |
| 它推荐的药（状态「计划中」，备注写明需医生确认、还没开始吃） | 不自动保存，要点一下才写入；已有同名的药不再写 |
| 对已有记录的修改 | 不自动保存，要点一下才改 |

**接口、隐私和日志**

- 接口要兼容 OpenAI 的 Chat Completions，并且支持工具调用（function calling；看图还需要对方支持图片）。地址、密钥和模型名在「连接 AI」页的最上面填，也可以在「AI助手」或「设置」里填。密钥只存在这台电脑上，不放进备份。
- 发给接口的内容：你说的话和图片、已有健康问题、用药、打卡项的名称，以及它回答时查到的记录。语音用 Mac 自己的听写，声音不上传。
- 每段对话记在数据目录的 `chats.json`，助手里点「历史」可以打开以前的对话。
- 每次回答在数据目录的 `assistant.log` 里记一段，方便查问题和看 token 花在哪：你说的话、每次请求的 token、调用了哪些工具和参数、工具返回的开头一段、回答和整理出的卡片、出错信息。这些和 `chats.json` 一样只在这台电脑上，不进备份；系统提示只记字数，图片只记类型和大小，不记密钥。在「设置 → AI助手」或助手的「修改接口」里点「查看日志」可以随时看最近的部分，最近的在最前面，也可以只看出错的。
- 浏览器里打开 `serve` 时可以打字和发图片，听写只在 Mac 窗口里。

**整理出来的内容不能代替医生的诊断。**

### 通过 MCP 接入

健康足迹通过 [MCP](https://modelcontextprotocol.io) 接入 AI：15 个工具（查询、录入、化验单导入、趋势、打卡、附件等），加 5 个分析模板。App 的「连接 AI」页会显示带着当前程序路径的配置，可以直接复制。

<details open>
<summary><b>Claude 桌面版</b>（设置 → 开发者 → 编辑配置）</summary>

```json
{
  "mcpServers": {
    "health-trail": { "command": "/Applications/健康足迹.app/Contents/MacOS/health-trail", "args": ["mcp"] }
  }
}
```

改完用 ⌘Q 完全退出 Claude 再打开。

</details>

<details>
<summary><b>Claude Code</b></summary>

```bash
claude mcp add health-trail -s user -- "/Applications/健康足迹.app/Contents/MacOS/health-trail" mcp
```

</details>

<details>
<summary><b>Codex</b>（命令行和编辑器插件共用）</summary>

```bash
codex mcp add health-trail -- "/Applications/健康足迹.app/Contents/MacOS/health-trail" mcp
```

</details>

<details>
<summary><b>其他支持 MCP 的 AI</b>（Cursor、Cherry Studio 等）</summary>

添加一个 stdio 类型的 MCP 服务器：命令填上面的程序路径，参数填 `mcp`。或者用 HTTP 地址 `http://127.0.0.1:8721/mcp`，这种方式需要 App 开着。

</details>

**分析模板**（在 Claude 桌面版的「＋」菜单里选用）：

| 模板 | 做什么 |
| :-- | :-- |
| **解读最近的化验单** | 哪些指标异常、和上次比怎么变了、该问医生什么 |
| **整理就诊摘要** | 把一段时间的症状、检查、用药整理成给医生看的摘要 |
| **分析症状** | 症状出现的规律和可能相关的记录 |
| **健康状况总评** | 目前所有健康问题、指标和用药的总览 |
| **打卡复盘** | 打卡完成情况和趋势 |

AI 只依据你的数据说话，遇到异常会建议你去看医生。**它的分析不能代替医生的诊断。**

## 工作方式

```mermaid
flowchart LR
  ai["Claude · Codex<br>其他 MCP 客户端"] -- "MCP（stdio / HTTP）" --> app
  win["Mac 窗口<br>菜单栏"] --> app
  app["健康足迹<br>Go 程序"] --> db[("SQLite<br>+ 附件")]
  app -. 提醒 .-> notify["系统通知"]
  app -. 同步 .-> ios["日历 · 提醒事项<br>→ iPhone"]
  app -. 备份 .-> backup["你选的文件夹<br>（zip）"]
  app -. AI助手 .-> api["你填的接口<br>（OpenAI 兼容）"]
```

一个 Go 程序同时提供窗口（Wails v3，系统自带的 WebView + React）、本机服务（`127.0.0.1:8721`）和 MCP。AI 用 stdio 接入时，每次会启动一个程序副本，和窗口读写同一个数据目录。

## 隐私与安全

- **数据只在你的电脑上**：默认在 `~/Library/Application Support/HealthVault`，可以用 `-data` 参数或环境变量 `HEALTH_TRAIL_DATA` 换地方。App 不收集使用统计。联网只有两处：向 GitHub 查有没有新版本（每隔几个小时一次，窗口回到前台时距上次超过半小时也会查，只问版本号，可以在「设置 → 软件更新」里关掉）；以及你在「AI助手」里自己填的接口。用「AI助手」时，你说的话、少量已有记录的名称，以及它回答时查到的记录会发给这个接口。声音用 Mac 本机识别，不上传。密钥存在数据目录的 `assistant.json`，不放进备份。
- **AI 会看到它读取的记录**：AI 读取记录时，相关内容会发给它的服务商处理（Claude 发给 Anthropic，Codex 发给 OpenAI）。用哪个 AI、让它读什么，由你决定。
- **本地服务只对本机开放**：默认只监听 `127.0.0.1`；本机浏览器里的其他网站无法读写数据，服务会拒绝 DNS 重绑定和跨站请求。
- **附件不会被当成网页运行**：只有图片、PDF 和纯文本能在页面里直接打开，其他类型一律作为下载返回。
- **AI 存附件也有限制**：只能存图片、PDF、Word/Excel、DICOM 片子、视频和录音，不存纯文本（以防有人诱导 AI 把密钥之类的文本存进档案），不读隐藏文件和「资源库」里的文件，错误信息里也不带文件路径。

## 命令行

```bash
./health-trail              # 打开窗口（同时在 127.0.0.1:8721 提供本地服务）
./health-trail serve        # 不开窗口，只运行服务（比如放在常开的 Mac mini 上）
./health-trail mcp          # MCP stdio 模式，给 AI 用
./health-trail info         # 显示数据目录和连接 AI 的配置
```

## 从源码构建

需要 Go 1.27+、Node 24+ 和 Xcode 命令行工具。

```bash
make app      # 生成 健康足迹.app，拖进「应用程序」即可
make bin      # 只生成命令行程序 ./health-trail
make test     # go vet、go test、前端类型检查
make dmg      # 同时支持 Apple 芯片和 Intel 的 dmg（发布时用 Developer ID 签名）
```

开发时用 `make dev`：界面在 http://localhost:5173 ，改前端保存后马上生效；后端在 `127.0.0.1:8722`，改了 `.go` 文件会自动重新编译和重启。数据用 `~/Library/Application Support/HealthTrail-Test` 里的测试数据（`make test-window` 打开的测试窗口也用它），不会碰真实数据。

想用自己的真实数据边用边调试，用 `make dev-real`：每次启动前会先把数据库和附件备份到 `~/Library/Application Support/HealthVault-dev-backups/<时间>`，保留最近 10 份。

代码结构和改代码的约定见 [CLAUDE.md](CLAUDE.md)，踩过的坑记在 [LESSONS.md](LESSONS.md)。每次推送都会由 [CI](.github/workflows/ci.yml) 跑测试、打包和 MCP 端到端测试；推送 `v*` 标签会自动签名、公证并发布，见 [RELEASING.md](RELEASING.md)。

## 计划

- 手机浏览器访问（功能已写好，暂时关闭，改好后开放）
- 手机上的 AI App（Claude、ChatGPT）访问：需要公网隧道和 OAuth 登录
- 在 App 里直接问 Claude
- 导入 Apple「健康」的数据
- 数据加密
- Windows 版

## 许可

[MIT](LICENSE)
