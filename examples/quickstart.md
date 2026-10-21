# 这就是用 SubSweep 的样子

三行命令，从样例账单到续费体检报告：

```console
$ go run . parse internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv
internal/bills/testdata/alipay_sample.csv（支付宝 · 共 27 笔交易，疑似代扣 21 笔）
  上海恩空中文化传播有限公司  支付宝  ¥25.00  2025-04-10  命中服务名单、按月重复扣款、备注代扣字样  0.95
  ...

$ go run . report internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv -o 体检报告.md
扫描 2 份账单、49 笔交易，检出疑似自动续费 8 项（35 笔扣款），年化合计 ¥2,052.00
已导出 Markdown 报告：体检报告.md

$ go run . serve
SubSweep 续费体检已启动：http://127.0.0.1:8787
数据仅在内存中，Ctrl+C 退出。
```

把 `internal/bills/testdata/*.csv` 换成你自己的支付宝 / 微信账单导出，就是一次完整的续费体检。
