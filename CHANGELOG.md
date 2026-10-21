# Changelog

All notable changes to SubSweep are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and versions follow
[Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-10-21

First release: the full local flow — parse, confirm, report, export.

### Added

- Bill export parsing for both platforms with automatic format and encoding
  detection: Alipay (GB18030, metadata preamble) and WeChat (UTF-8 with BOM,
  padded rows); unknown formats and empty input are rejected.
- Renewal-candidate detection as a union of three heuristics — embedded
  50-service catalog hit, same-amount monthly repetition (27–33 day gaps),
  and category/remark deduction keywords — each with a confidence weight,
  for per-item user confirmation rather than mechanical verdicts.
- Synthetic golden fixtures shipped with the repo: `alipay_sample.csv`
  (GB18030) and `wechat_sample.csv` (UTF-8 BOM), covered by parser tests.
- Human-verified service knowledge base (`internal/catalog/data/services.json`):
  50 high-frequency renewal services with canonical names, billing aliases,
  distinctive keywords and official cancel paths (video, music, cloud storage,
  productivity, e-commerce memberships).
- Merchant matching: dictionary hits plus fuzzy candidates; optional
  DeepSeek classification assist (`deepseek-flash` by default, model id
  configurable) — off by default, sends merchant names only, and can never
  invent cancel paths.
- Renewal audit report: signings grouped per platform/merchant/amount,
  annualized cost (monthly × 12, each signing counted once), suspected-
  forgotten ranking, Markdown / terminal table / JSON rendering.
- Local web UI served by the same binary (`subsweep serve`, 127.0.0.1:8787):
  drag-and-drop import, per-item confirm / unrecognized / undo, merchant
  correction via candidates, Markdown / single-page HTML / JSON export;
  data lives in memory only.
- CLI: `subsweep parse`, `subsweep report [-o file]`, `subsweep serve`,
  `subsweep version`.
- Release CI building linux/darwin/windows × amd64/arm64 archives with
  checksums via goreleaser; manual demo-GIF refresh workflow (vhs).
- Recorded demo: `docs/demo-results.json` (inputs, commands, real output,
  limitations) and the recorded `assets/demo.gif` with its `docs/demo.tape`.

### Known limitations

- Monthly-repeat detection requires two consecutive charges 27–33 days
  apart; renewals that skip a month can be missed.
- Cancel paths can go stale after app redesigns; the catalog is
  community-maintained — PRs welcome.
- Export-format changes can break parsing; failing samples become new
  golden tests.
