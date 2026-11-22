# Privacy Policy / 隐私政策

**Effective Date / 生效日期:** 2026-11-22  
**App:** Health Trail - 健康足迹 (`com.chunbin1.healthtrail`)  
**Developer / 开发者:** Chunbin Li  
**Contact / 联系方式:** https://github.com/chunbin1/health-trail/issues

---

## English

Health Trail ("the App") is a private, offline-first personal health record organizer. Your privacy is fundamental to how the App is designed.

### 1. Data We Collect
**We do not collect, track, sell, or share any personal or health data.**
- All health records (conditions, symptoms, visits, medications, appointments, lab reports, body metrics, check-ins, break timer logs, and attachments) are stored locally in an SQLite database inside your device's private storage (`Application Support/HealthTrail`).
- **Excluded from iCloud / System Backups:** The App marks its local data directory (`Application Support/HealthTrail`, which contains `health.db`, attachments, and any local AI API key) with `NSURLIsExcludedFromBackupKey` so your sensitive health records and keys are never automatically uploaded to iCloud or system backups.
- The App contains no third-party analytics, no advertising SDKs, and no tracking telemetry.

### 2. Optional User-Configured Network Features
By default, the App works 100% offline. Network requests are only made when you explicitly enable and configure one of the following optional features:
- **Optional AI Assistant (Bring Your Own Key):** If you choose to configure your own OpenAI-compatible API endpoint and API key (such as Zhipu GLM or another provider you select), the messages and photos you submit to the AI Assistant are sent directly from your device to your chosen API provider under that provider's privacy policy. Your API key is stored locally on your device.
- **Optional Home Computer Mode:** If you choose to connect to your own self-hosted Health Trail desktop server over local Wi-Fi (`http://192.168.x.x:port`), Tailscale MagicDNS (`http://*.ts.net:port`), or HTTPS (`tailscale serve`), requests go directly between your iPhone and your own computer. Your access token is stored in the iOS Keychain, and no developer-operated relay server is involved.

### 3. Permissions
The App only requests system permissions when you use a related feature:
- **Camera / Photo Library:** To attach photos of lab reports or prescriptions to your local records.
- **Local Network:** Only when connecting to your own Health Trail desktop server on your local Wi-Fi.
- **Microphone / Speech Recognition (Strictly On-Device):** Optional voice dictation for the AI Assistant runs strictly on-device (`requiresOnDeviceRecognition = true`). Audio is never uploaded to Apple or any remote server; on devices or languages that do not support on-device offline speech recognition, in-app dictation is unavailable rather than falling back to cloud processing.

### 4. Data Control & Deletion
Because all data is stored locally on your device and excluded from iCloud/system backups, you have full control at all times. You can edit or delete any record inside the App, and uninstalling the App permanently removes all local data.

### 5. Medical Disclaimer
Health Trail is a personal record-keeping tool and does not provide medical advice, diagnosis, or treatment. Always consult a qualified healthcare professional regarding any medical condition.

---

## 简体中文

「Health Trail - 健康足迹」（以下简称「本应用」）是一款本地优先的个人健康档案整理工具。保护您的隐私是本应用的核心设计原则。

### 1. 我们收集的数据
**我们不收集、不追踪、不出售也不共享您的任何个人或健康数据。**
- 您的全部健康记录（健康问题、症状、就诊、用药、预约、检查报告与检验指标、身体数据、打卡、休息计时与附件）均仅保存在您设备本机的私有存储空间（`Application Support/HealthTrail` 下的 SQLite 数据库）中。
- **不进入 iCloud / 系统备份**：本应用对本地数据目录（`Application Support/HealthTrail`，内含 `health.db`、附件及本机 AI 助手密钥）设置了排除备份标记（`NSURLIsExcludedFromBackupKey`），不会自动上传到 iCloud 或系统备份。
- 本应用不含任何第三方统计分析、广告 SDK 或行为追踪代码。

### 2. 可选的用户自配网络功能
本应用默认完全离线运行。仅当您主动配置并使用以下可选功能时，才会产生网络请求：
- **可选 AI 助手（自带 API Key）**：若您自行配置了 OpenAI 兼容接口地址与个人 API Key（如智谱或其他您指定的服务商），您发送给 AI 助手的文字或化验单图片将由您的设备直接发往您所选的服务商，并适用该服务商的隐私政策。您的 API Key 仅保存在本机。
- **可选连接家用电脑模式**：若您自行配置连接自己的 Health Trail 桌面端服务，支持同一局域网 Wi-Fi（`http://192.168.x.x:端口`）、Tailscale MagicDNS（`http://*.ts.net:端口`）或 HTTPS（`tailscale serve`）。访问令牌保存在 iOS 钥匙串（Keychain）中，数据仅在您的手机与您的电脑之间直接传输，不经过任何开发者服务器。

### 3. 系统权限说明
本应用仅在您使用对应功能时请求权限：
- **相机 / 相册**：用于拍摄或选取检查报告、处方照片并保存到本机记录。
- **本地网络**：仅在您主动连接同一 Wi-Fi 下的自有桌面端时使用。
- **麦克风 / 语音识别（仅限设备端离线识别）**：AI 助手的语音听写强制启用设备端离线识别（`requiresOnDeviceRecognition = true`），语音音频绝不上传至苹果或任何外部服务器；若当前设备或语言不支持端侧离线识别，应用内听写会提示不可用而不会回退到云端识别。

### 4. 数据控制与删除
所有数据均存储在您的设备本地且排除了 iCloud / 系统备份，您可以随时在应用内编辑或删除任意记录；卸载应用将永久清除本机全部数据。

### 5. 医疗免责声明
本应用仅用于个人健康记录整理，不构成任何医疗建议、诊断或治疗方案。涉及医疗健康决策，请务必咨询专业医生或医疗机构。
