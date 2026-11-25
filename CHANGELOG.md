# Changelog

All notable changes to SubSweep are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/) and versions follow
[Semantic Versioning](https://semver.org/).

## [0.2.0] - 2026-11-26

Correctness release: the detection heuristics behave the same whatever order the
export lists its rows in, a transaction-free export can no longer pass as a clean
bill, and the local UI can no longer be bound to a network interface. No new
features — the scope is the two promises the tool makes: it reads your export
correctly, and it stays on your machine.

### Fixed

- Monthly-repeat detection no longer depends on the export's row order.
  `hasMonthlyCadence` compared only adjacent rows in file order, so a
  newest-first export produced negative gaps and the signal silently never
  fired: the shipped sample fixtures report 8 suspected signings / 35 charges /
  ¥2,052.00 as recorded, but the identical rows listed newest-first reported
  7 / 30 / ¥1,584.00, both at exit 0. Charges are now sorted by date before the
  27–33 day window is applied; the window itself is unchanged.
- A statement that carries the official header but no transaction rows — an
  interrupted or truncated export — is now rejected with an error. It used to
  parse into an empty statement, so `subsweep report` printed "0 笔交易 /
  未检出" at exit 0, indistinguishable from a genuinely clean bill.
- `subsweep serve` now refuses a non-loopback `-addr`. `-addr 0.0.0.0:8787`
  used to bind every interface, exposing the upload endpoint, the per-item
  confirmation state and the optional LLM key endpoint to the LAN while the
  flag help and the README promised 仅本机回环. `127.0.0.1`, `::1` and
  `localhost` still work; `-addr :8787` is refused because an empty host means
  all interfaces.

### Changed

- The binary's version is read from the embedded `VERSION` file instead of a
  second literal in `main.go`; goreleaser still stamps it with
  `-ldflags -X main.version`. A test keeps `VERSION` and the recorded demo
  (`docs/demo-results.json`) in lockstep.
- The README no longer promises a hosted no-login web version for v0.2: this
  repository contains the local tool only, and the pricing tiers are labelled as
  planned with the form and timing left open.

### Known limitations

- Monthly-repeat detection requires two consecutive charges 27–33 days apart
  after sorting by date; renewals that skip a month can still be missed.
- Cancel paths can go stale after app redesigns; the catalog is
  community-maintained — PRs welcome.
- Export-format changes can break parsing; failing samples become new golden
  tests.

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
