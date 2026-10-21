**English** | [简体中文](./README.md)

<div align="center">

<img src="https://readme-typing-svg.demolab.com?font=Noto+Sans+SC:wght@600&size=26&duration=3200&pause=1000&color=0E7490&center=true&vCenter=true&width=920&height=82&lines=Drop%20your%20Alipay%20%2F%20WeChat%20bill%20export%3BSee%20every%20auto-renewal%20in%2030%20seconds%3BWhat%20it%20costs%20a%20year%20%E2%80%94%20and%20where%20to%20cancel" alt="Drop your Alipay / WeChat bill export; See every auto-renewal in 30 seconds; What it costs a year — and where to cancel" />

# SubSweep · Renewal Audit

**The auto-renewals you signed inside Alipay and WeChat are easy to forget. Hand SubSweep the official bill exports and get a renewal audit checklist within 10 minutes: what each recurring charge is, what it costs per year, and where to turn it off.**

[![Release](https://img.shields.io/github/v/release/SuperMarioYL/subsweep)](https://github.com/SuperMarioYL/subsweep/releases/latest)
[![CI](https://github.com/SuperMarioYL/subsweep/actions/workflows/ci.yml/badge.svg)](https://github.com/SuperMarioYL/subsweep/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/macOS%20%7C%20Windows%20%7C%20Linux-passing-blue)](./.github/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-059669.svg)](./LICENSE)

**One local binary · no account · nothing uploaded · no database**

</div>

---

## Why SubSweep exists

The trouble with auto-renewals is not the amount — it is that **you can no longer remember what you signed up for**:

- The "auto-deduction" management pages in Alipay and WeChat each list only **their own platform's** agreements — what you signed on the other platform, and what it costs per year, is invisible from either side;
- Every app buries its cancellation entry deep in settings and leans on "cancelling membership ≠ cancelling auto-renewal" style retention copy, so most people give up halfway;
- Expense-tracking apps import bills to categorize spending; they never answer "which of these are still-active deduction agreements, and where do I cancel each one?"

SubSweep fills exactly that gap: **official bill export → heuristic detection → your per-item confirmation → annualized cost + the official cancel path for each item**. The output is an audit *checklist*, not a mechanical verdict — detection is a candidate list for you to verify, and the cancellation itself is always performed by you, on the official page.

| | Alipay/WeChat "auto-deduction" pages | Expense-tracking apps | Manual subscription sheets (e.g. Wallos) | SubSweep |
| --- | --- | --- | --- | --- |
| Scope | Own-platform agreements only | Depends on bill import | Manual entry | Both platform exports together |
| Finding renewals | Lists signed agreements | Expense categorization only | Maintained by hand | Three detection heuristics + your confirmation |
| Annualized cost | Not provided | Spending statistics | Yes, but hand-entered | Automatic: monthly fee × 12, each signing once |
| Official cancel paths | Own-platform unsubscription entry | None | None | Per-item in-app path + web entry |

## Quickstart

**Option 1: download a prebuilt binary** (macOS / Windows / Linux, a single ~10MB file) — grab the archive for your platform from [Releases](https://github.com/SuperMarioYL/subsweep/releases/latest) and unpack it.

**Option 2: run from source** (requires Go 1.24+):

```bash
git clone https://github.com/SuperMarioYL/subsweep.git
cd subsweep

# See it work on the bundled sample bills first (or drop in your own exports)
go run . parse internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv
go run . report internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv -o report.md

# Local web UI: drop in bills, confirm item by item, export the report (opens your browser)
go run . serve
```

**Exporting your own bills** (the only step that takes more than a minute, done inside the official apps):

- Alipay: Me → Bills → "…" (top right) → Issue transaction statement → for personal reconciliation → CSV arrives by email;
- WeChat: Me → Services → Wallet → Bills → "FAQ" (top right) → Download bill → for personal reconciliation → CSV arrives by email.

Drop both CSVs into the local page opened by `subsweep serve` — Alipay and WeChat exports can be imported together.

## Demo

![demo](assets/demo.gif)

A recorded run of the full flow (the data is the anonymized synthetic sample shipped with the repo, fully reproducible; the CLI output itself is in Chinese, with the cancel entry of each service named):

```console
$ ./subsweep report internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv
续费体检清单（2026-10-21 08:25）
扫描 2 份账单、49 笔交易，检出疑似自动续费 8 项（35 笔扣款），年化合计 ¥2,052.00
已确认 0 项、待确认 8 项、不认识 0 项

商户                      平台      月费     年化  信号                                        置信度  状态    取消入口      
杭州优卖网络科技有限公司  支付宝  ¥39.00  ¥468.00  按月重复扣款                                  0.30  待确认  未收录        
腾讯视频                  微信    ¥30.00  ¥360.00  命中服务名单、按月重复扣款、备注含代扣字样    0.95  待确认  腾讯视频 App  
爱奇艺                    支付宝  ¥25.00  ¥300.00  命中服务名单、按月重复扣款、备注含代扣字样    0.95  待确认  爱奇艺 App    
喜马拉雅                  支付宝  ¥20.00  ¥240.00  命中服务名单、按月重复扣款、备注含代扣字样    0.95  待确认  喜马拉雅 App  
QQ音乐                    微信    ¥15.00  ¥180.00  命中服务名单、按月重复扣款、备注含代扣字样    0.95  待确认  QQ音乐 App    
百度网盘                  支付宝  ¥15.00  ¥180.00  命中服务名单、按月重复扣款                    0.85  待确认  百度网盘 App  
网易云音乐                微信    ¥15.00  ¥180.00  命中服务名单、按月重复扣款                    0.85  待确认  网易云音乐 App
WPS超级会员               支付宝  ¥12.00  ¥144.00  命中服务名单、备注含代扣字样                  0.70  待确认  WPS Office App

待核实 8 项——逐项确认后即为最终清单。
```

Read as: scanned 2 statements / 49 transactions; 8 suspected auto-renewals found (35 charges); ¥2,052.00 annualized; 0 confirmed, 8 pending. The first row (杭州优卖网络科技有限公司) is a recurring charge from a merchant the catalog does not know yet — exactly the kind of signing worth checking first.

<details>
<summary><strong>Full record: docs/demo-results.json (inputs, commands, output, limitations)</strong></summary>

```json
{
  "version": "0.1.0",
  "inputs": [
    { "path": "internal/bills/testdata/alipay_sample.csv", "encoding": "GB18030", "transactions": 27 },
    { "path": "internal/bills/testdata/wechat_sample.csv", "encoding": "UTF-8 (BOM)", "transactions": 22 }
  ],
  "results": {
    "suspect_signings": 8,
    "suspect_charges": 35,
    "annualized_total": "¥2,052.00",
    "forgotten": 8
  },
  "limitations": [
    "Detection is a union of heuristics (service list + same-amount monthly repeat + category/remark keywords) — a checklist to confirm, not a mechanical verdict",
    "Monthly-repeat detection requires two consecutive charges 27–33 days apart; renewals that skip a month can be missed",
    "Keywords are scanned only in the transaction-category and remark columns; \"连续包月\" in the product-description column is not a signal",
    "Cancel paths come from a human-verified catalog (50 services); app redesigns can make a path stale — always defer to the provider's official page",
    "The DeepSeek assist is off by default; when enabled it sends merchant names only — never amounts or account identifiers"
  ]
}
```

</details>

## How it works

```
subsweep —— one Go binary; embed.FS packs the web UI and the service catalog
├── bills parser   format detection (Alipay GB18030 / WeChat UTF-8), renewal-candidate heuristics
├── catalog        embedded services.json: official cancel paths for 50 high-frequency services (human-verified)
├── match          dictionary hit → fuzzy match → optional DeepSeek classification (off by default)
├── report         annualized cost, suspected-forgotten ranking, Markdown / terminal table / JSON rendering
└── local server   net/http 127.0.0.1:8787 + embedded single-page UI, data in memory only
```

Official exports contain **no column that directly marks a charge as an auto-renewal**: the transaction category is just a spending taxonomy, and in WeChat even subscription renewals show up as plain merchant purchases. So SubSweep does no single-column mechanical filtering — it takes the union of three heuristics and hands the suspected rows to you for verification:

1. **Known-service hit** — the merchant name (including billing company names and membership product names) matches the built-in catalog of 50 renewal services;
2. **Same-amount monthly repeat** — the same merchant charging the same amount with 27–33 days between consecutive charges;
3. **Category/remark keywords** — the transaction category or remark column mentions wording like 代扣 (deduction agreement) or 自动续费 (auto-renewal).

Signals carry confidence weights of 0.55 / 0.30 / 0.15 (capped at 0.95); the checklist becomes final only after your per-item confirmation. In the local web UI, uncertain rows can be corrected by picking a candidate service, or marked "unrecognized". Exports: Markdown, single-page HTML, and JSON.

## Privacy and boundaries

- **Nothing leaves your machine**: a single local binary; bills live in memory and the local session only — no account, no upload, no database;
- **Official exports only**: no reverse engineering of local encrypted databases, no scraping, no automated actions on your behalf;
- **You do the cancelling**: SubSweep only points at official paths; the cancellation is always performed by you on the provider's page;
- **The optional LLM has a boundary**: the DeepSeek assist is off by default; when enabled it sends merchant names only (never amounts, dates, or account identifiers), and cancel paths always come from the human-verified catalog — the model never generates them.

## Configuration

Optional flags of `subsweep serve`:

| Flag | Default | Description |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8787` | Listen address (loopback only) |
| `-no-open` | `false` | Do not open the browser automatically |
| `-llm-key` | empty (or env `SUBSWEEP_LLM_KEY`) | Optional DeepSeek API key, enables the classification assist |
| `-llm-model` | `deepseek-flash` | Optional model id (`deepseek-flash` / `deepseek-v4-pro`) |
| `-llm-url` | `https://api.deepseek.com` | Optional OpenAI-compatible endpoint |

## Pricing

The CLI and the local web UI of SubSweep are **open source and free** — this repository is the whole product. The ongoing cost is maintaining the cancel-path catalog, and revenue follows the path below (planned, shipping with the v0.2 no-login web version):

| | Free checklist | Deep report ¥9.9 | Family bundle ¥19.9 |
| --- | --- | --- | --- |
| Renewal audit checklist | ✓ | ✓ | ✓ |
| Monthly cash-flow forecast | — | ✓ | ✓ |
| Cancellation priority + service-desk scripts | — | ✓ | ✓ |
| Family multi-account merge | — | — | ✓ |

The web version targets people who cannot run a command line: no login, single session, bills used and discarded, nothing stored. ¥9.9 is the price of a cup of milk tea — less than a single forgotten renewal's monthly fee. The local CLI core stays open source; no referral commissions or affiliate integrations — neutral cancellation guidance is the whole point of this tool.

## Roadmap

- [x] v0.1 — both-platform bill parsing, heuristic detection, 50-service cancel-path catalog, annualized report, local web UI, Markdown / HTML / JSON export
- [ ] Cancel-path catalog expansion and quarterly re-verification (community PRs against `internal/catalog/data/services.json`)
- [ ] No-login web version (single session, used and discarded) with the deep report
- [ ] Family multi-account merge
- [ ] Tracking export-format drift: failed-parse samples become new golden tests

**Explicitly out of scope**: unattended automated cancellation, bill upload or cloud storage, parsing platforms other than Alipay/WeChat, LLM-generated cancel steps.

## Known limitations

- Detection is a union of heuristics, meant for per-item confirmation rather than a mechanical verdict; renewals that skip a month can be missed;
- Cancel paths come from a human-verified catalog and can go stale after app redesigns — please open an issue or send a PR when one does;
- Export-format changes can break parsing; issues with an anonymized failing sample are welcome, and each sample becomes a new test case.

## Contributing

Issues and PRs are welcome. The two most valuable kinds of contribution are failed-parse samples and catalog additions or corrections (`internal/catalog/data/services.json`). A Gitee mirror is published in sync for faster access in mainland China.

<p align="center"><sub><a href="./LICENSE">MIT</a> © 2026 SuperMarioYL</sub></p>
