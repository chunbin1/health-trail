# 上 Mac App Store：进度和待办

（2026-10-25 整理。**同一天决定先不上商店**：沙盒里要用户一个个授权文件夹，体验不好，改发 Developer ID 签名、公证过的 dmg（`make dmg` / `make notarize`，CI 见 `.github/workflows/ci.yml`）。下面留作以后再上商店时的参考；「允许读取的文件夹」在沙盒外不显示，留着无害。）

## 已经完成

**苹果网站上（都做完了）**
- 开发者会员已生效，团队 ID `XU4K6FP3HC`（LI CHUNBIN）。
- 证书已装进「登录」钥匙串并有效：`Apple Distribution: LI CHUNBIN (XU4K6FP3HC)`、`3rd Party Mac Developer Installer: LI CHUNBIN (XU4K6FP3HC)`，2027-10-22 到期。私钥要导出成 .p12 备份（用户自己做）。
- App ID：`com.chunbin1.healthtrail`（Health Trail）。
- 描述文件：「Health Trail App Store」（Mac App Store Connect），在 `~/Downloads/Health_Trail_App_Store.provisionprofile`，2027-10-22 到期。打包时放进 `Contents/embedded.provisionprofile`，不用双击。
- App Store Connect：已同意服务条款，App 记录已建好：名称「Health Trail - 健康足迹」（Health Trail 被占用），主要语言英语（美国），SKU `healthtrail-mac`，Apple ID `6826733377`，macOS 1.0「准备提交」。欧盟「交易商状态」没声明（先只上美区）。

**沙盒验证（2026-10-25，结论：可以上）**
- 沙盒里窗口、本机服务、HTTP MCP 都正常。
- Claude/Codex 从外面启动的 stdio MCP 也在沙盒里，系统会把 HOME 改到容器，默认数据目录自动落在 `~/Library/Containers/<bundle id>/Data/Library/Application Support/HealthVault`，和窗口是同一份数据，双向读写都通过。
- 问题：沙盒里 MCP 读不了容器外的文件 → 已加「下载」只读权限（commit `18e974c`）。
- 坑：在 `Contents/MacOS/` 里另放的可执行文件直接运行会 SIGTRAP（exit 133），主程序不会。
- 坑：沙盒里没有 `network.client` 时窗口白屏（WKWebView 需要它），已加进 `build/appstore.entitlements`（LESSONS #30）。补上后用户确认测试版窗口显示正常。

## 这次改了、还没提交的（「允许读取的文件夹」）

让用户在设置里授权文件夹（安全书签），Claude 就能存那里的附件：
- `bookmarks_darwin.go`：创建 / 打开安全书签（cgo）。
- `folders.go`：授权存在数据目录 `folders.json`；窗口启动、MCP 启动时都打开书签；`/api/folders`（GET/POST/DELETE）只给窗口用（网络上 404）。
- `main.go`：接上授权；测试版（bundle id 不是正式的）日历写进「健康足迹（测试）」。
- `internal/mcpserver/attach.go`：沙盒提示改成「下载 + 设置里添加过的文件夹」；`~/Library` 检查同时看真实主目录（沙盒里 HOME 是容器）。
- `frontend/src/pages/Settings.tsx` 等：设置页「允许读取的文件夹」，只在沙盒版显示。
- `Makefile`：`make appstore-test` 打出 `健康足迹（App Store 测试版）.app`（bundle id `com.chunbin1.healthtrail.appstoretest`，用 `build/appstore.entitlements`，本机临时签名，git 忽略）。
- `make test` 全绿。**还没验证书签在真实点击下能不能用。**

## 接下来要做

1. **验证授权文件夹（要用户点）**：打开测试版 →「设置 → 允许读取的文件夹 → 添加…」→ 选「桌面」。然后用逐条发送的 stdio 驱动跑测试版的 MCP（`…/Contents/MacOS/health-trail mcp`），`add_attachment` 一个桌面上的测试 PDF，应当成功；退出重开测试版、再跑一次 MCP，确认书签重启后仍有效。通过后提交。
2. **备份位置也用书签**：沙盒里手填路径、「一键用 iCloud」都不行，只能「选择…」；选完要存书签，备份循环和 MCP 进程启动时打开。可以复用 `folders.go` 的书签函数。
3. **数据迁移**：正式沙盒版第一次启动时，数据在容器里是空的。用户的真实数据在 `~/Library/Application Support/HealthVault`，沙盒读不到 → 第一次启动时引导用户在对话框里选这个文件夹授权，然后复制进容器（或者提示用备份恢复）。
4. **版本号**：`CFBundleShortVersionString` 改成 `1.0.0`，`CFBundleVersion` 用递增的整数构建号（现在是 git 提交号，商店会拒）。
5. **Info.plist 补字段**：`LSApplicationCategoryType`（`public.app-category.healthcare-fitness` 或 `medical`）、`ITSAppUsesNonExemptEncryption = false`。
6. **`make appstore`**：通用二进制（arm64 + x86_64，`lipo`）、嵌入描述文件、用 `Apple Distribution` 证书 + `build/appstore.entitlements` 签名、`productbuild --sign "3rd Party Mac Developer Installer: …"` 打 `.pkg`。签名只用钥匙串里证书的名字，不碰私钥。
7. **Transporter**：用户从 Mac App Store 安装；上传 .pkg → Mac 版 TestFlight 自己先装一遍试。
8. **还没测的沙盒功能（要用户点）**：日历/提醒事项授权弹窗、通知、开机启动（SMAppService）、附件上传和打开、手机访问（局域网弹窗）。
9. **商店页面**：截图（2880×1800 / 2560×1600 / 1440×900 / 1280×800）、描述、关键词、隐私政策网址（必填）、App 隐私标签（不收集数据）、年龄分级、价格。
10. **之后**：中国大陆区要 App 备案；欧盟要声明交易商状态；`mcp-attachments` 分支要合回 `main`。

## 注意

- 永远不要用真实数据目录测试（CLAUDE.md）；测试版有自己的容器，可以随便用。
- 用 `open -n …app --args -port 87xx` 启动测试版（LESSONS #25），不要占 8721。
- 不替用户输入 Apple 账号密码、不碰证书私钥；苹果网站上的操作用户在 Chrome 里登录，可以用 Claude in Chrome 帮着填，最后一步提交前先确认。
