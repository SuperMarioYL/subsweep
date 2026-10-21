[English](./README.en.md) | **简体中文**

<div align="center">

<img src="https://readme-typing-svg.demolab.com?font=Noto+Sans+SC:wght@600&size=26&duration=3200&pause=1000&color=0E7490&center=true&vCenter=true&width=920&height=82&lines=%E6%8B%96%E5%85%A5%E6%94%AF%E4%BB%98%E5%AE%9D%20%2F%20%E5%BE%AE%E4%BF%A1%E8%B4%A6%E5%8D%95%E5%AF%BC%E5%87%BA%3B30%20%E7%A7%92%E7%9C%8B%E6%B8%85%E5%85%A8%E9%83%A8%E8%87%AA%E5%8A%A8%E7%BB%AD%E8%B4%B9%3B%E4%B8%80%E5%B9%B4%E8%8A%B1%E5%A4%9A%E5%B0%91%E3%80%81%E5%8E%BB%E5%93%AA%E9%87%8C%E5%85%B3%E6%8E%89" alt="拖入支付宝 / 微信账单导出；30 秒看清全部自动续费；一年花多少、去哪里关掉" />

# SubSweep · 续费体检

**在支付宝和微信里签过的自动续费，你未必都记得。把官方账单导出交给 SubSweep，10 分钟内得到一份续费体检清单：每一笔代扣是什么、一年花多少、去哪里关掉。**

[![Release](https://img.shields.io/github/v/release/SuperMarioYL/subsweep)](https://github.com/SuperMarioYL/subsweep/releases/latest)
[![CI](https://github.com/SuperMarioYL/subsweep/actions/workflows/ci.yml/badge.svg)](https://github.com/SuperMarioYL/subsweep/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/macOS%20%7C%20Windows%20%7C%20Linux-passing-blue)](./.github/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-059669.svg)](./LICENSE)

**单文件本地程序 · 无账号 · 无上传 · 无数据库**

</div>

---

## 为什么做 SubSweep

自动续费的问题不在于"扣了多少"，在于**你记不全签过哪些**：

- 支付宝和微信的"自动扣款"管理页只列**本平台**的签约项——另一个平台签了什么、每项年化多少，两边互相看不见；
- 各 App 的取消入口层级很深，还常用"关闭会员 ≠ 关闭自动续费"的话术挽留，找到入口的成本高到多数人放弃；
- 记账 App 导入账单是为了统计支出，它不回答"哪几笔是仍在生效的代扣协议、去哪里关"。

SubSweep 补的是这段空隙：**官方账单导出 → 启发式检出 → 你逐项确认 → 年化成本 + 逐项官方取消路径**。它是"体检清单"而不是机械结论——检出结果由你逐项核实，取消动作永远由你本人在官方页面完成。

| | 支付宝/微信"自动扣款"页 | 记账 App | 手动订阅表（如 Wallos） | SubSweep |
| --- | --- | --- | --- | --- |
| 汇总范围 | 仅本平台签约项 | 取决于账单导入 | 手动录入 | 两平台导出一起看 |
| 自动发现代扣 | 列出签约项 | 只做支出分类 | 需要手动维护 | 三类启发式检出 + 你逐项确认 |
| 年化成本 | 不提供 | 支出统计口径 | 有，但手填 | 自动按月费 × 12 折算 |
| 官方取消路径 | 本平台解约入口 | 无 | 无 | 逐项 App 内路径 + 在线入口 |

## 快速开始

**方式一：下载预编译二进制**（macOS / Windows / Linux，约 10MB 单文件）——到 [Releases](https://github.com/SuperMarioYL/subsweep/releases/latest) 下载对应平台压缩包，解压即用。

**方式二：源码运行**（需要 Go 1.24+）：

```bash
git clone https://github.com/SuperMarioYL/subsweep.git
cd subsweep

# 先用随仓库的样例账单看效果（也可以直接换成你自己的导出）
go run . parse internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv
go run . report internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv -o 体检报告.md

# 本地网页：拖入账单、逐项确认、导出报告（自动打开浏览器）
go run . serve
```

**导出你自己的账单**（全流程唯一超过 60 秒的一步，在官方 App 内完成）：

- 支付宝：我的 → 账单 → 右上角「…」→ 开具交易流水证明 → 用于个人对账 → 邮箱接收 CSV；
- 微信：我 → 服务 → 钱包 → 账单 → 右上角「常见问题」→ 下载账单 → 用于个人对账 → 邮箱接收 CSV。

两份 CSV 拖进 `subsweep serve` 打开的本地页面即可，支付宝与微信的导出可以一起拖入。

## 效果演示

![demo](assets/demo.gif)

下面是一次完整运行的实录（数据为随仓库内置的脱敏合成样例，可完整复现）：

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

<details>
<summary><strong>完整实录：docs/demo-results.json（输入、命令、输出、局限）</strong></summary>

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
    "检出是启发式并集（服务名单 + 同额月度重复 + 分类/备注关键词），供逐项确认而非机械结论",
    "月度重复判定要求某相邻两次扣款间隔 27-33 天；跳月续费可能漏检",
    "关键词只扫描 交易分类/交易类型 与 备注 两列，商品说明列的“连续包月”不算信号",
    "取消路径来自人工核验的知识库（50 项），App 改版后路径可能过期，请以服务方官方页面为准",
    "DeepSeek 辅助分类默认关闭；开启后仅发送商户名，不含金额与账号"
  ]
}
```

</details>

## 工作原理

```
subsweep —— 单个 Go 二进制，embed.FS 打包网页与服务知识库
├── bills parser   格式探测（支付宝 GB18030 / 微信 UTF-8）、疑似代扣行启发式检出
├── catalog        内嵌 services.json：50 项高频服务的官方取消路径（人工核验）
├── match          字典命中 → 模糊匹配 → 可选 DeepSeek 分类（默认关闭）
├── report         年化成本折算、疑似遗忘项排序、Markdown / 终端表格 / JSON 渲染
└── local server   net/http 127.0.0.1:8787 + 内嵌单页 UI，数据仅在内存
```

官方导出里**没有任何一列直接标记"代扣"**：交易分类只是消费品类目，微信里订阅续期也只显示为"商户消费"。所以 SubSweep 不做单列机械过滤，而是三类启发式取并集，把疑似代扣行交给用户逐项核实：

1. **命中服务名单** —— 商户名（含计费公司名与会员产品名）命中内置的 50 项续费服务知识库；
2. **同额月度重复** —— 同一商户、同一金额，出现间隔 27–33 天的连续扣款；
3. **分类/备注关键词** —— 交易分类或备注字段出现"代扣 / 自动续费 / 自动扣款"等字样。

每个信号给出 0.55 / 0.30 / 0.15 的置信度权重（上限 0.95），逐项确认后清单才成为最终结论。本地网页里拿不准的行可以点选候选服务纠正匹配，或标注"不认识"；导出支持 Markdown、单页 HTML 与 JSON。

## 隐私与边界

- **数据不出本机**：单文件本地运行，账单只在内存与本地会话中，无账号、无上传、无数据库；
- **只用官方导出**：不逆向本地加密数据库、不爬取、不代操作；
- **取消由你完成**：SubSweep 只给官方路径指引，取消动作永远由用户本人在官方页面执行；
- **可选 LLM 的边界**：DeepSeek 辅助默认关闭；开启后仅发送商户名（不含金额、日期、账号），且取消路径永远来自人工核验的知识库，不由模型生成。

## 配置

`subsweep serve` 的可选参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-addr` | `127.0.0.1:8787` | 监听地址（仅本机回环） |
| `-no-open` | `false` | 启动后不自动打开浏览器 |
| `-llm-key` | 空（也可用环境变量 `SUBSWEEP_LLM_KEY`） | 可选：DeepSeek API Key，开启辅助分类 |
| `-llm-model` | `deepseek-flash` | 可选：模型 id（如 `deepseek-flash` / `deepseek-v4-pro`） |
| `-llm-url` | `https://api.deepseek.com` | 可选：OpenAI 兼容接口地址 |

## 定价

SubSweep 的命令行与本地网页是**开源免费**的，这份源码就是全部。持续成本在取消路径知识库的维护上，收入按以下路径展开（规划中，随 v0.2 免登录网页版推出）：

| | 免费清单 | 深度报告 ¥9.9 | 家庭合并 ¥19.9 |
| --- | --- | --- | --- |
| 续费体检清单 | ✓ | ✓ | ✓ |
| 月度现金流预测 | — | ✓ | ✓ |
| 取消优先级排序 + 客服话术包 | — | ✓ | ✓ |
| 家庭多账单合并 | — | — | ✓ |

网页版面向装不了命令行的普通用户：免登录、单次会话、账单即用即弃、不存储。¥9.9 是一杯奶茶的价格，低于任何一笔被遗忘续费的月费。本地命令行版本体持续开源，不做推荐返佣与会员导流——"中立的取消指引"是这个工具的立身之本。

## 路线图

- [x] v0.1 —— 两平台账单解析、启发式检出、50 项取消路径知识库、年化报告、本地网页、Markdown / HTML / JSON 导出
- [ ] 取消路径知识库扩充与季度复核（社区 PR 直改 `internal/catalog/data/services.json`）
- [ ] 免登录网页版（单次会话、即用即弃）与深度报告
- [ ] 家庭多账单合并
- [ ] 导出格式变化追踪：解析失败样本直接进金样测试

**明确不做**：平台自动化与无人值守代取消、账单上传与云端存储、支付宝/微信之外平台的账单解析、LLM 生成取消步骤。

## 已知限制

- 检出是启发式并集，供逐项确认而非机械结论；跳月续费（隔月扣一次）可能漏检；
- 取消路径来自人工核验的知识库，App 改版后可能过期——发现过期请提 issue 或直接 PR；
- 导出格式变化会导致解析失败，欢迎附上失败样本（脱敏后）提 issue，样本会变成新的测试用例。

## 参与贡献

欢迎 issue 与 PR：解析失败样本、知识库新增服务与路径纠错（`internal/catalog/data/services.json`）是最有价值的两类贡献。Gitee 镜像同步发布，国内访问更快。

<p align="center"><sub><a href="./LICENSE">MIT</a> © 2026 SuperMarioYL</sub></p>
