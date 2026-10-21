// Command subsweep turns official Alipay/WeChat bill exports into a renewal
// audit: suspected auto-renewal charges, annualized cost and official cancel
// paths — fully local, one binary.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/SuperMarioYL/subsweep/internal/bills"
	"github.com/SuperMarioYL/subsweep/internal/catalog"
	"github.com/SuperMarioYL/subsweep/internal/match"
	"github.com/SuperMarioYL/subsweep/internal/report"
	"github.com/SuperMarioYL/subsweep/internal/server"
	"golang.org/x/text/width"
)

//go:embed web
var webFS embed.FS

// version is stamped by goreleaser via -ldflags "-X main.version=...".
var version = "0.1.0"

const usage = `subsweep %s — 续费体检：把支付宝/微信账单导出变成续费体检清单

用法：
  subsweep parse  <账单.csv>...            在终端打印疑似代扣候选行
  subsweep report <账单.csv>... [-o 报告.md]  生成续费体检清单（终端表格 + 可选 Markdown）
  subsweep serve  [-addr 127.0.0.1:8787]   启动本地网页，拖入账单、逐项确认、导出报告
  subsweep version                         查看版本

示例：
  subsweep parse internal/bills/testdata/alipay_sample.csv
  subsweep report internal/bills/testdata/alipay_sample.csv internal/bills/testdata/wechat_sample.csv -o 体检报告.md
  subsweep serve

账单从哪里来：
  支付宝 App：我的 → 账单 → 右上角「…」→ 开具交易流水证明（用于个人对账，CSV）
  微信 App：我 → 服务 → 钱包 → 账单 → 右上角「常见问题」→ 下载账单（用于个人对账，CSV）
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "parse":
		err = runParse(os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("subsweep %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	case "help", "-h", "--help":
		fmt.Printf(usage, version)
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n%s", os.Args[1], fmt.Sprintf(usage, version))
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

/* ---------- parse ---------- */

func runParse(args []string) error {
	set := flag.NewFlagSet("parse", flag.ExitOnError)
	if err := set.Parse(args); err != nil {
		return err
	}
	if set.NArg() == 0 {
		return errors.New("parse 需要至少一个账单 CSV 文件")
	}

	var firstErr error
	for _, path := range set.Args() {
		st, err := readStatement(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s：%v\n", path, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		suspects := st.Suspects()
		fmt.Printf("\n%s（%s · 共 %d 笔交易，疑似代扣 %d 笔）\n",
			path, platformLabel(st.Platform), len(st.Deductions), len(suspects))
		if len(suspects) == 0 {
			fmt.Println("  未检出疑似自动续费扣款。")
			continue
		}
		rows := make([][]string, 0, len(suspects))
		for _, s := range suspects {
			rows = append(rows, []string{
				s.Deduction.RawMerchant,
				platformLabel(s.Deduction.Platform),
				fmt.Sprintf("¥%.2f", float64(s.Deduction.AmountFen)/100),
				s.Deduction.Date,
				signalText(s.Signals),
				fmt.Sprintf("%.2f", s.Confidence),
			})
		}
		printTable([]string{"商户", "平台", "金额", "日期", "命中信号", "置信度"}, rows, map[int]bool{2: true, 5: true}, "  ")
	}
	return firstErr
}

/* ---------- report ---------- */

func runReport(args []string) error {
	// -o is scanned manually: users naturally write "report 文件... -o 报告.md"
	// and Go's flag package stops at the first non-flag argument.
	var outPath string
	var files []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 >= len(args) {
				return errors.New("-o 需要一个输出文件路径")
			}
			outPath = args[i+1]
			i++
		case "-h", "--help":
			fmt.Printf(usage, version)
			return nil
		default:
			files = append(files, args[i])
		}
	}
	if len(files) == 0 {
		return errors.New("report 需要至少一个账单 CSV 文件")
	}

	var statements []*bills.Statement
	var suspects []bills.Suspect
	for _, path := range files {
		st, err := readStatement(path)
		if err != nil {
			return fmt.Errorf("%s：%w", path, err)
		}
		statements = append(statements, st)
		suspects = append(suspects, st.Suspects()...)
	}

	rep := report.Build(report.Input{
		Statements: statements,
		Suspects:   suspects,
		Catalog:    match.Catalog{KB: catalog.Default()},
		Now:        time.Now(),
	})

	fmt.Print(rep.Table())
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(rep.Markdown()), 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", outPath, err)
		}
		fmt.Printf("\n已导出 Markdown 报告：%s\n", outPath)
	}
	return nil
}

/* ---------- serve ---------- */

func runServe(args []string) error {
	set := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := set.String("addr", server.DefaultAddr, "监听地址（仅本机回环）")
	noOpen := set.Bool("no-open", false, "启动后不自动打开浏览器")
	llmKey := set.String("llm-key", os.Getenv("SUBSWEEP_LLM_KEY"), "可选 DeepSeek API Key（也可用环境变量 SUBSWEEP_LLM_KEY）；默认关闭")
	llmModel := set.String("llm-model", match.DefaultModel, "可选：LLM 模型 id（如 deepseek-flash / deepseek-v4-pro）")
	llmURL := set.String("llm-url", match.DefaultBaseURL, "可选：OpenAI 兼容接口地址")
	if err := set.Parse(args); err != nil {
		return err
	}

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	srv := server.New(server.Options{
		Addr: *addr,
		KB:   catalog.Default(),
		Web:  web,
		LLM: match.LLMConfig{
			Enabled: *llmKey != "",
			APIKey:  *llmKey,
			Model:   *llmModel,
			BaseURL: *llmURL,
		},
	})

	httpServer := &http.Server{Addr: srv.Addr(), Handler: srv.Handler()}
	errCh := make(chan error, 1)
	go func() {
		if lerr := httpServer.ListenAndServe(); lerr != nil && !errors.Is(lerr, http.ErrServerClosed) {
			errCh <- lerr
		}
		close(errCh)
	}()

	url := "http://" + srv.Addr()
	fmt.Printf("SubSweep 续费体检已启动：%s\n数据仅在内存中，Ctrl+C 退出。\n", url)
	if *llmKey != "" {
		fmt.Printf("DeepSeek 辅助分类已开启（模型 %s，仅发送商户名）。\n", *llmModel)
	}
	if !*noOpen {
		openBrowser(url)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case s := <-sig:
		name := "中断信号"
		if s == os.Interrupt {
			name = "Ctrl+C"
		}
		fmt.Printf("\n收到 %s，正在退出…\n", name)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("（无法自动打开浏览器，请手动访问 %s）\n", url)
	}
}

/* ---------- shared helpers ---------- */

func readStatement(path string) (*bills.Statement, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return bills.Parse(raw)
}

func platformLabel(p bills.Platform) string {
	switch p {
	case bills.PlatformAlipay:
		return "支付宝"
	case bills.PlatformWechat:
		return "微信"
	default:
		return string(p)
	}
}

var signalLabels = map[string]string{
	bills.SignalKnownMerchant: "命中服务名单",
	bills.SignalMonthlyRepeat: "按月重复扣款",
	bills.SignalKeyword:       "备注代扣字样",
}

func signalText(signals []string) string {
	out := make([]string, 0, len(signals))
	for _, s := range signals {
		if label, ok := signalLabels[s]; ok {
			out = append(out, label)
		} else {
			out = append(out, s)
		}
	}
	return strings.Join(out, "、")
}

// printTable pads cells to the widest cell per column, counting East Asian
// wide runes as two columns so CJK text stays aligned in the terminal.
func printTable(header []string, rows [][]string, rightAlign map[int]bool, sep string) {
	w := make([]int, len(header))
	for i, cell := range header {
		w[i] = displayWidth(cell)
	}
	for _, row := range rows {
		for i, cell := range row {
			if cw := displayWidth(cell); cw > w[i] {
				w[i] = cw
			}
		}
	}
	write := func(row []string) {
		var b strings.Builder
		for i, cell := range row {
			gap := w[i] - displayWidth(cell)
			if rightAlign[i] {
				b.WriteString(strings.Repeat(" ", gap))
				b.WriteString(cell)
			} else {
				b.WriteString(cell)
				b.WriteString(strings.Repeat(" ", gap))
			}
			if i < len(row)-1 {
				b.WriteString(sep)
			}
		}
		fmt.Println(b.String())
	}
	write(header)
	for _, row := range rows {
		write(row)
	}
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			w += 2
		default:
			w++
		}
	}
	return w
}
