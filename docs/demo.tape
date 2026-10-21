Output assets/demo.gif

Set FontFamily "Menlo, PingFang SC, Noto Sans Mono CJK SC, DejaVu Sans Mono, monospace"
Set Theme {
	"name": "subsweep",
	"background": "#0B1220",
	"foreground": "#E8EEF7",
	"cursor": "#7DD3FC",
	"black": "#0B1220",
	"green": "#4ADE80",
	"yellow": "#FDE047",
	"blue": "#60A5FA",
	"magenta": "#F472B6",
	"cyan": "#7DD3FC",
	"white": "#E8EEF7"
}
Set Width 1520
Set Height 760
Set FontSize 17
Set Padding 42
Set TypingSpeed 16ms

# 检出：拖入微信账单导出，逐行打印疑似代扣候选
Sleep 1.2s
Type './subsweep parse internal/bills/testdata/wechat_sample.csv'
Enter
Sleep 5.5s
Ctrl+L
Sleep 0.8s

# 体检：两份账单一起看，按签约分组给出年化成本与官方取消入口
Type './subsweep report internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv'
Enter
Sleep 7.5s
Ctrl+L
Sleep 0.8s

# 本地网页：拖入账单、逐项确认、导出报告（浏览器自动打开）
Type './subsweep serve --no-open'
Enter
Sleep 4.5s
Ctrl+C
Sleep 1.2s
