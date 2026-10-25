<div align="center">

[中文](README.md) | **English**

<img src="frontend/public/icon.svg" width="112" alt="Health Trail">

# 健康足迹 · Health Trail

### Your health records, kept on your own computer.

**Medical history, lab reports, medications, body metrics and daily check-ins, all in one place.**<br>
Let Claude or Codex enter and analyze them for you. Your data never leaves your Mac.

[![Release](https://img.shields.io/github/v/release/chunbin1/health-vault?include_prereleases&label=release&color=111111)](https://github.com/chunbin1/health-vault/releases) [![CI](https://img.shields.io/github/actions/workflow/status/chunbin1/health-vault/ci.yml?label=CI&color=111111)](https://github.com/chunbin1/health-vault/actions/workflows/ci.yml) [![License](https://img.shields.io/badge/license-MIT-111111)](LICENSE)<br>
![macOS](https://img.shields.io/badge/macOS_13+-000000?logo=apple&logoColor=white) ![MCP](https://img.shields.io/badge/MCP-15_tools-111111) ![SQLite](https://img.shields.io/badge/SQLite-local-003B57?logo=sqlite&logoColor=white)

**[Download](https://github.com/chunbin1/health-vault/releases)** &nbsp;·&nbsp; **[Connect your AI](#connect-your-ai)** &nbsp;·&nbsp; **[Build from source](#build-from-source)** &nbsp;·&nbsp; **[Releasing](RELEASING.md)** (Chinese)

<br>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/overview-dark.png">
  <img src="docs/img/overview-light.png" width="900" alt="The Today page: active health issues, today's medications, today's check-ins, upcoming appointments and results to watch">
</picture>

<sub>The screenshots show the Chinese interface with made-up sample data. The app can also be switched to English.</sub>

</div>

<br>

## Is it for you?

Lab reports sit in your photo library, medical records in a hospital app, you rely on memory for when you took your pills, and at a follow-up you can't recall what your last result was. Health Trail puts all of this in a local database and hands it to an AI you trust to help keep it in order:

- **Snap a photo to log it**: send a screenshot of a lab report to Claude and it enters every item, flags what's high or low, and saves the original image as an attachment.
- **Ask about trends any time**: "How has my uric acid changed over the last two years?" "Has my CRP come down?" The AI answers from your data, not from guesses.
- **Reminders on time**: for medications, check-ins and follow-ups, your Mac shows a notification, and they can sync to the Calendar and Reminders apps on your iPhone.
- **Data stays on your Mac**: one SQLite file plus an attachments folder, backed up to a place you choose.

It's for people who want to manage their own (or their family's) health records seriously but don't want to hand medical records to a cloud service. If you only need to track steps and sleep, the Health app built into your iPhone is enough.

For now it supports macOS 13 and later only, on both Apple silicon and Intel.

## Quick start

**1 · Install**: download `HealthTrail-<version>.dmg` from **[Releases](https://github.com/chunbin1/health-vault/releases)**, open it, and drag 健康足迹 (Health Trail) into Applications.

<sub>The dmg is signed with a Developer ID and notarized by Apple, so it opens with a double-click. You can also [build from source](#build-from-source). When a new version comes out, the app tells you in Settings → Software update, and one click updates it.</sub>

**2 · Connect AI**: open the app's Connect AI page and copy the configuration for the AI you use (Claude Desktop, Claude Code, Codex, or any other AI that supports MCP).

**3 · Start using it**: say to the AI

```text
Enter this lab report            (attach the screenshot)
I took my medication this morning
Help me list the questions to ask the doctor at this follow-up
```

## What it does

### Records

| Type | What it records |
| :-- | :-- |
| **Health issues** | Illnesses you're dealing with, allergies, chronic conditions; symptoms, visits and medications can all be filed under one |
| **Symptoms** | When, where, how severe |
| **Visits** | Hospital, department, doctor, diagnosis, doctor's advice |
| **Medications** | Dose, frequency, the times of day you take it; tap once to log a dose, and the pill count adds up automatically |
| **Reports** | Checkups, lab tests, imaging; each lab result is saved individually with its reference range as printed, and you can see the trend of any single result |
| **Body metrics** | Weight, blood pressure, blood glucose, or any metric you add yourself |
| **Appointments** | Follow-ups and tests, with the place and what to prepare (fasting, bring your last scans) |
| **Check-ins** | "Weigh in after waking up", "Keep to my sleep schedule", with streaks and completion rates |

Reports, prescriptions and scans can be stored as attachments under the matching record, and opened with a click in the window.

<table>
<tr>
<td width="50%">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/trend-dark.png">
  <img src="docs/img/trend-light.png" alt="Result trends: four uric acid results, the reference range and the trend line">
</picture>
</td>
<td width="50%">
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/metrics-dark.png">
  <img src="docs/img/metrics-light.png" alt="Body metrics: a 90-day weight chart and the change over the period">
</picture>
</td>
</tr>
</table>

### Reminders and syncing to iPhone

| | |
| :-- | :-- |
| **Medications** | Reminds you if a dose hasn't been logged by the set time; no reminder if you already took and logged it |
| **Check-ins** | Reminds you if a check-in isn't checked off by its reminder time |
| **Appointments** | One reminder at 8 PM the evening before and one before it starts (1 hour by default) |
| **iPhone** | Appointments go into Calendar and medications into Reminders, synced to your phone and watch via iCloud; tick off a medication reminder on your iPhone and the app logs a dose |

When you close the window, the app keeps running in the menu bar, so reminders and automatic backups carry on; it can also open at login. Calendar entries carry a `healthtrail://` link that takes you back to that record in the app with one click.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/img/checkins-dark.png">
  <img src="docs/img/checkins-light.png" width="760" alt="Check-ins: today's check-in items, the last 30 days of completion, and streaks">
</picture>

### Backups

Pick a folder (iCloud Drive or an external drive is a good choice). Each backup is a zip containing the database and attachments, and the most recent 30 are kept by default. Backups can run automatically daily, weekly, after changes, or on quit; click Restore in the list to go back to that point in time. Your current data is saved separately before a restore.

### Chinese and English

The interface can switch between Chinese and English; your data is always stored exactly as entered.

## Connect your AI

Health Trail connects to AI through [MCP](https://modelcontextprotocol.io): 15 tools (querying, entering records, importing lab reports, trends, check-ins, attachments and more), plus 5 analysis templates. The app's Connect AI page shows configurations with the current program path filled in, ready to copy.

<details open>
<summary><b>Claude Desktop</b> (Settings → Developer → Edit Config)</summary>

```json
{
  "mcpServers": {
    "health-trail": { "command": "/Applications/健康足迹.app/Contents/MacOS/health-trail", "args": ["mcp"] }
  }
}
```

After editing, quit Claude completely with ⌘Q and open it again.

</details>

<details>
<summary><b>Claude Code</b></summary>

```bash
claude mcp add health-trail -s user -- "/Applications/健康足迹.app/Contents/MacOS/health-trail" mcp
```

</details>

<details>
<summary><b>Codex</b> (shared by the CLI and the editor extension)</summary>

```bash
codex mcp add health-trail -- "/Applications/健康足迹.app/Contents/MacOS/health-trail" mcp
```

</details>

<details>
<summary><b>Other AIs that support MCP</b> (Cursor, Cherry Studio and others)</summary>

Add a stdio MCP server: set the command to the program path above and the argument to `mcp`. Or use the HTTP address `http://127.0.0.1:8721/mcp`; this way the app needs to be open.

</details>

**Analysis templates** (pick one from the "+" menu in Claude Desktop; they appear under their Chinese names):

| Template | What it does |
| :-- | :-- |
| **Review the latest lab report** (解读最近的化验单) | Which results are abnormal, how they changed since last time, what to ask the doctor |
| **Prepare a visit summary** (整理就诊摘要) | Turns a period's symptoms, tests and medications into a summary for your doctor |
| **Analyze a symptom** (分析症状) | Patterns in when a symptom appears, and records that may be related |
| **Overall health review** (健康状况总评) | An overview of all current health issues, results and medications |
| **Check-in review** (打卡复盘) | How your check-ins went and how they're trending |

The AI only speaks from your data, and suggests seeing a doctor when something looks abnormal. **Its analysis is no substitute for a doctor's diagnosis.**

## How it works

```mermaid
flowchart LR
  ai["Claude · Codex<br>other MCP clients"] -- "MCP (stdio / HTTP)" --> app
  win["Mac window<br>menu bar"] --> app
  app["Health Trail<br>Go program"] --> db[("SQLite<br>+ attachments")]
  app -. reminders .-> notify["System notifications"]
  app -. sync .-> ios["Calendar · Reminders<br>→ iPhone"]
  app -. backups .-> backup["Folder you choose<br>(zip)"]
```

A single Go program provides the window (Wails v3, the system WebView + React), the local server (`127.0.0.1:8721`) and MCP. When an AI connects over stdio, it starts its own copy of the program each time, which reads and writes the same data folder as the window.

## Privacy and security

- **Your data stays on your computer**: by default in `~/Library/Application Support/HealthVault`; you can move it with the `-data` flag or the `HEALTH_TRAIL_DATA` environment variable. The app itself uploads nothing and collects no usage statistics. Its only network access is checking GitHub once a day for a new version (it only asks for the version number, and you can turn this off in Settings → Software update).
- **The AI sees the records it reads**: when an AI reads records, their content is sent to its provider for processing (Claude to Anthropic, Codex to OpenAI). Which AI you use and what you let it read is up to you.
- **The local server is open to this Mac only**: by default it listens only on `127.0.0.1`; other websites in your browser can't read or write your data, and the server rejects DNS rebinding and cross-site requests.
- **Attachments never run as web pages**: only images, PDFs and plain text open directly in the page; every other type is returned as a download.
- **What the AI can store as attachments is limited too**: only images, PDFs, Word/Excel files, DICOM scans, video and audio recordings. It won't store plain text (so no one can trick the AI into filing text such as keys into your records), won't read hidden files or files in Library, and leaves file paths out of its error messages.

## Command line

```bash
./health-trail              # Open the window (also serves locally at 127.0.0.1:8721)
./health-trail serve        # Run only the server, no window (e.g. on an always-on Mac mini)
./health-trail mcp          # MCP stdio mode, for AI clients
./health-trail info         # Show the data folder and the configuration for connecting AI
```

## Build from source

Requires Go 1.27+, Node 24+ and the Xcode Command Line Tools.

```bash
make app      # Build 健康足迹.app; drag it into Applications
make bin      # Build only the command-line program ./health-trail
make test     # go vet, go test, frontend type check
make dmg      # dmg for both Apple silicon and Intel (signed with a Developer ID for releases)
```

For development, use `make dev`: the UI runs at http://localhost:5173 and frontend changes take effect as soon as you save; the backend runs at `127.0.0.1:8722` and recompiles and restarts automatically when a `.go` file changes. Data lives in a temporary folder, so your real data is never touched.

To debug while using your own real data, use `make dev-real`: before each start it backs up the database and attachments to `~/Library/Application Support/HealthVault-dev-backups/<time>`, keeping the most recent 10.

The code structure and coding conventions are in [CLAUDE.md](CLAUDE.md) (Chinese), and pitfalls we've hit are recorded in [LESSONS.md](LESSONS.md) (Chinese). Every push runs the tests, packaging and the MCP end-to-end test in [CI](.github/workflows/ci.yml); pushing a `v*` tag signs, notarizes and publishes a release automatically, see [RELEASING.md](RELEASING.md) (Chinese).

## Roadmap

- Access from a phone browser (already built, temporarily turned off; will open once it's polished)
- Access from AI apps on the phone (Claude, ChatGPT): needs a public tunnel and OAuth sign-in
- Ask Claude directly inside the app
- Import data from Apple Health
- Data encryption
- A Windows version

## License

[MIT](LICENSE)
