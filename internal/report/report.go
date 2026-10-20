// Package report turns flagged renewal candidates into the 续费体检清单
// (renewal audit checklist): per-signing annualized cost, the signals and
// confidence behind each detection, the user's confirmation state, and the
// official cancel path from the curated catalog where the merchant is
// known. Rendering targets are Markdown (shareable archive), an aligned
// terminal table, and JSON (consumed by the local web UI).
package report

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/SuperMarioYL/subsweep/internal/bills"
	"golang.org/x/text/width"
)

// schemaV1 tags JSON snapshots so exported reports stay identifiable
// as the format evolves.
const schemaV1 = "subsweep.report.v1"

// Status is the user's verdict on one detected charge, keyed by row id.
type Status string

const (
	StatusPending      Status = "pending"      // not yet reviewed by the user
	StatusConfirmed    Status = "confirmed"    // user recognizes this renewal
	StatusUnrecognized Status = "unrecognized" // user does not recognize this charge
)

// label is the Chinese display label used by the renderers.
func (s Status) label() string {
	switch s {
	case StatusConfirmed:
		return "已确认"
	case StatusUnrecognized:
		return "不认识"
	default:
		return "待确认"
	}
}

// CancelPath is the human-verified official way to turn off one service's
// auto-renewal. Paths come only from the curated catalog — never generated.
type CancelPath struct {
	App   string   `json:"app"`
	Steps []string `json:"steps"`
	URL   string   `json:"url"`
}

// Service is the catalog knowledge a report line needs for a matched
// merchant: the canonical service name and its official cancel path.
type Service struct {
	CanonicalName string     `json:"canonical_name"`
	Cancel        CancelPath `json:"cancel"`
}

// Catalog resolves a raw bill merchant name to a known renewal service.
// internal/catalog implements it over the embedded services.json; tests
// substitute an in-memory stub.
type Catalog interface {
	Find(merchant string) (Service, bool)
}

// Item is one line of the checklist: a group of bill rows that belong to
// the same signing agreement (same platform, merchant identity and amount).
// The annualized cost is the monthly charge times twelve — the detection
// heuristics target monthly renewals, and one signing is counted once no
// matter how many of its rows appear in the export window.
type Item struct {
	Platform      bills.Platform `json:"platform"`
	Merchant      string         `json:"merchant"`     // canonical name when catalog-matched, else raw merchant
	RawMerchant   string         `json:"raw_merchant"` // merchant string as it appears on the bill
	AmountFen     int64          `json:"amount_fen"`   // monthly charge
	AnnualizedFen int64          `json:"annualized_fen"`
	FirstDate     string         `json:"first_date"` // YYYY-MM-DD
	LastDate      string         `json:"last_date"`
	Charges       int            `json:"charges"`    // occurrences inside the export window
	Signals       []string       `json:"signals"`    // union across rows, in canonical order
	Confidence    float64        `json:"confidence"` // max across rows, 0..1
	Status        Status         `json:"status"`
	Service       *Service       `json:"service,omitempty"` // nil when the catalog does not know the merchant
	RowIDs        []string       `json:"row_ids"`
}

// Report is the 续费体检清单 snapshot.
type Report struct {
	Schema        string    `json:"schema"`
	GeneratedAt   time.Time `json:"generated_at"`
	Statements    int       `json:"statements"`   // bill exports scanned
	Transactions  int       `json:"transactions"` // deduction rows scanned
	Items         []*Item   `json:"items"`
	AnnualizedFen int64     `json:"annualized_total_fen"` // total, each signing counted once
	Forgotten     []*Item   `json:"forgotten"`            // items not confirmed as known renewals
}

// Input assembles what Build needs. Catalog and Statuses may be nil; rows
// without an entry in Statuses count as StatusPending. Statuses is keyed
// by deduction RowID.
type Input struct {
	Statements []*bills.Statement
	Suspects   []bills.Suspect
	Catalog    Catalog
	Statuses   map[string]Status
	Now        time.Time
}

// Build groups flagged rows into signing-agreement items, resolves catalog
// services, applies confirmation state and computes the annualized total.
// It never fails: empty input produces an empty report.
func Build(in Input) *Report {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	// Empty slices, not nil, so JSON snapshots carry "items": [] rather
	// than null — friendlier for the web UI.
	r := &Report{Schema: schemaV1, GeneratedAt: now, Items: []*Item{}, Forgotten: []*Item{}}
	for _, st := range in.Statements {
		r.Statements++
		r.Transactions += len(st.Deductions)
	}

	// Catalog lookups are cached per raw merchant string; a match also
	// unifies name variants of the same service into one series.
	resolved := map[string]*Service{}
	serviceFor := func(raw string) *Service {
		if svc, ok := resolved[raw]; ok {
			return svc
		}
		var svc *Service
		if in.Catalog != nil {
			if got, ok := in.Catalog.Find(raw); ok {
				svc = &got
			}
		}
		resolved[raw] = svc
		return svc
	}

	type seriesKey struct {
		platform bills.Platform
		merchant string
		amount   int64
	}
	series := map[seriesKey]*Item{}
	seenRow := map[string]bool{}

	for _, sus := range in.Suspects {
		d := sus.Deduction
		if seenRow[d.RowID] {
			continue // one bill row must never be counted twice
		}
		seenRow[d.RowID] = true

		svc := serviceFor(d.RawMerchant)
		merchant := d.RawMerchant
		if svc != nil {
			merchant = svc.CanonicalName
		}
		k := seriesKey{d.Platform, merchant, d.AmountFen}

		it := series[k]
		if it == nil {
			it = &Item{
				Platform:      d.Platform,
				Merchant:      merchant,
				RawMerchant:   d.RawMerchant,
				AmountFen:     d.AmountFen,
				AnnualizedFen: d.AmountFen * 12,
				FirstDate:     d.Date,
				LastDate:      d.Date,
				Service:       svc,
			}
			series[k] = it
			r.Items = append(r.Items, it)
		}
		it.Charges++
		it.RowIDs = append(it.RowIDs, d.RowID)
		if d.Date != "" && d.Date < it.FirstDate {
			it.FirstDate = d.Date
		}
		if d.Date > it.LastDate {
			it.LastDate = d.Date
		}
		if sus.Confidence > it.Confidence {
			it.Confidence = sus.Confidence
		}
		for _, sig := range sus.Signals {
			if !slices.Contains(it.Signals, sig) {
				it.Signals = append(it.Signals, sig)
			}
		}
	}

	for _, it := range r.Items {
		it.Signals = orderSignals(it.Signals)
		sort.Strings(it.RowIDs)
		it.Status = itemStatus(it, in.Statuses)
	}

	// Biggest annual cost first; the remaining keys only break ties.
	sort.Slice(r.Items, func(i, j int) bool {
		a, b := r.Items[i], r.Items[j]
		if a.AnnualizedFen != b.AnnualizedFen {
			return a.AnnualizedFen > b.AnnualizedFen
		}
		if a.Merchant != b.Merchant {
			return a.Merchant < b.Merchant
		}
		return a.Platform < b.Platform
	})

	for _, it := range r.Items {
		r.AnnualizedFen += it.AnnualizedFen
	}

	// 疑似遗忘：anything the user has not confirmed as a known renewal.
	// Unrecognized charges rank ahead of unreviewed ones.
	forgotten := make([]*Item, 0, len(r.Items))
	for _, it := range r.Items {
		if it.Status != StatusConfirmed {
			forgotten = append(forgotten, it)
		}
	}
	sort.Slice(forgotten, func(i, j int) bool {
		a, b := forgotten[i], forgotten[j]
		if ra, rb := statusRank(a.Status), statusRank(b.Status); ra != rb {
			return ra < rb
		}
		if a.AnnualizedFen != b.AnnualizedFen {
			return a.AnnualizedFen > b.AnnualizedFen
		}
		return a.Merchant < b.Merchant
	})
	r.Forgotten = forgotten

	return r
}

// itemStatus aggregates the rows of one signing: an unrecognized row marks
// the whole series unrecognized; the series is confirmed only when every
// row is; anything else stays pending.
func itemStatus(it *Item, statuses map[string]Status) Status {
	unrecognized, confirmed := false, true
	for _, id := range it.RowIDs {
		switch rowStatus(id, statuses) {
		case StatusUnrecognized:
			unrecognized = true
		case StatusConfirmed:
		default:
			confirmed = false
		}
	}
	switch {
	case unrecognized:
		return StatusUnrecognized
	case confirmed && len(it.RowIDs) > 0:
		return StatusConfirmed
	default:
		return StatusPending
	}
}

// rowStatus reads one row's verdict, rejecting unknown values as pending.
func rowStatus(rowID string, statuses map[string]Status) Status {
	if s, ok := statuses[rowID]; ok {
		switch s {
		case StatusConfirmed, StatusUnrecognized:
			return s
		}
	}
	return StatusPending
}

func statusRank(s Status) int {
	switch s {
	case StatusUnrecognized:
		return 0
	case StatusConfirmed:
		return 2
	default:
		return 1
	}
}

// orderSignals puts the known detection signals in their canonical order
// and appends any unknown ones alphabetically, keeping output stable.
func orderSignals(signals []string) []string {
	known := []string{bills.SignalKnownMerchant, bills.SignalMonthlyRepeat, bills.SignalKeyword}
	var out, extra []string
	for _, k := range known {
		if slices.Contains(signals, k) {
			out = append(out, k)
		}
	}
	for _, s := range signals {
		if !slices.Contains(known, s) {
			extra = append(extra, s)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// Markdown renders the full shareable report.
func (r *Report) Markdown() string {
	var b strings.Builder
	c := r.counts()

	fmt.Fprintf(&b, "# 续费体检报告\n\n")
	fmt.Fprintf(&b, "生成时间：%s\n\n", r.GeneratedAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "扫描范围：%d 份账单、%d 笔交易；检出疑似自动续费 %d 项（%d 笔扣款）。\n\n",
		r.Statements, r.Transactions, len(r.Items), r.totalCharges())

	b.WriteString("## 概览\n\n")
	fmt.Fprintf(&b, "- 确认状态：已确认 %d 项、待确认 %d 项、不认识 %d 项\n", c.confirmed, c.pending, c.unrecognized)
	fmt.Fprintf(&b, "- 年化成本合计：**%s**（月费 × 12，同一签约只计一次）\n\n", formatFen(r.AnnualizedFen))

	if len(r.Forgotten) > 0 {
		b.WriteString("## 疑似遗忘项\n\n")
		fmt.Fprintf(&b, "以下 %d 项尚未确认为你已知悉的续费，请优先核实：\n\n", len(r.Forgotten))
		for i, it := range r.Forgotten {
			fmt.Fprintf(&b, "%d. **%s**（%s）— 月费 %s，年化 %s，状态：%s\n",
				i+1, it.Merchant, platformLabel(it.Platform),
				formatFen(it.AmountFen), formatFen(it.AnnualizedFen), it.Status.label())
		}
		b.WriteString("\n")
	}

	b.WriteString("## 逐项明细\n\n")
	if len(r.Items) == 0 {
		b.WriteString("未检出疑似自动续费扣款。\n")
	}
	for _, it := range r.Items {
		r.writeItem(&b, it)
	}

	b.WriteString("## 说明\n\n")
	b.WriteString("- 检出结果由启发式生成，是供逐项核实的候选清单，不是机械结论。\n")
	b.WriteString("- 年化成本按月费 × 12 折算；同一签约（同平台、同商户、同金额）只计一次。\n")
	b.WriteString("- 取消路径来自人工核验的知识库，若与最新版本不符，请以服务方官方页面为准；取消动作请由你本人在官方页面完成。\n")
	return b.String()
}

func (r *Report) writeItem(b *strings.Builder, it *Item) {
	fmt.Fprintf(b, "### %s（%s）\n\n", it.Merchant, platformLabel(it.Platform))
	fmt.Fprintf(b, "- 账单商户：%s\n", it.RawMerchant)
	if it.Charges == 1 {
		fmt.Fprintf(b, "- 月费 %s ｜ 年化 %s ｜ 扣款 1 笔（%s）\n",
			formatFen(it.AmountFen), formatFen(it.AnnualizedFen), it.FirstDate)
	} else {
		fmt.Fprintf(b, "- 月费 %s ｜ 年化 %s ｜ 扣款 %d 笔（%s 至 %s）\n",
			formatFen(it.AmountFen), formatFen(it.AnnualizedFen), it.Charges, it.FirstDate, it.LastDate)
	}
	fmt.Fprintf(b, "- 命中信号：%s\n", strings.Join(signalTexts(it.Signals), "、"))
	fmt.Fprintf(b, "- 置信度 %.2f ｜ 状态：%s\n", it.Confidence, it.Status.label())
	if it.Service != nil {
		cancel := it.Service.Cancel
		fmt.Fprintf(b, "- 官方取消路径：%s：%s\n", cancel.App, strings.Join(cancel.Steps, " → "))
		if cancel.URL != "" {
			fmt.Fprintf(b, "- 在线入口：%s\n", cancel.URL)
		}
	} else {
		b.WriteString("- 官方取消路径：知识库暂未收录该商户，请在其 App 内查找“自动续费管理”。\n")
	}
	b.WriteString("\n")
}

// Table renders the checklist as an aligned terminal table.
func (r *Report) Table() string {
	var b strings.Builder
	c := r.counts()

	fmt.Fprintf(&b, "续费体检清单（%s）\n", r.GeneratedAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "扫描 %d 份账单、%d 笔交易，检出疑似自动续费 %d 项（%d 笔扣款），年化合计 %s\n",
		r.Statements, r.Transactions, len(r.Items), r.totalCharges(), formatFen(r.AnnualizedFen))

	if len(r.Items) == 0 {
		b.WriteString("未检出疑似自动续费扣款。\n")
		return b.String()
	}

	fmt.Fprintf(&b, "已确认 %d 项、待确认 %d 项、不认识 %d 项\n\n", c.confirmed, c.pending, c.unrecognized)

	cancelEntry := func(it *Item) string {
		if it.Service != nil {
			return it.Service.Cancel.App
		}
		return "未收录"
	}
	header := []string{"商户", "平台", "月费", "年化", "信号", "置信度", "状态", "取消入口"}
	rightAlign := map[int]bool{2: true, 3: true, 5: true}
	rows := make([][]string, 0, len(r.Items))
	for _, it := range r.Items {
		rows = append(rows, []string{
			it.Merchant,
			platformLabel(it.Platform),
			formatFen(it.AmountFen),
			formatFen(it.AnnualizedFen),
			strings.Join(signalTexts(it.Signals), "、"),
			fmt.Sprintf("%.2f", it.Confidence),
			it.Status.label(),
			cancelEntry(it),
		})
	}
	b.WriteString(renderTable(header, rows, rightAlign))

	if c.pending+c.unrecognized > 0 {
		fmt.Fprintf(&b, "\n待核实 %d 项——逐项确认后即为最终清单。\n", c.pending+c.unrecognized)
	}
	return b.String()
}

// JSON renders the report as an indented snapshot for the local web UI
// and for export.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func (r *Report) totalCharges() int {
	n := 0
	for _, it := range r.Items {
		n += it.Charges
	}
	return n
}

type statusCounts struct{ confirmed, pending, unrecognized int }

func (r *Report) counts() statusCounts {
	var c statusCounts
	for _, it := range r.Items {
		switch it.Status {
		case StatusConfirmed:
			c.confirmed++
		case StatusUnrecognized:
			c.unrecognized++
		default:
			c.pending++
		}
	}
	return c
}

var signalLabels = map[string]string{
	bills.SignalKnownMerchant: "命中服务名单",
	bills.SignalMonthlyRepeat: "按月重复扣款",
	bills.SignalKeyword:       "备注含代扣字样",
}

func signalTexts(signals []string) []string {
	out := make([]string, 0, len(signals))
	for _, s := range signals {
		if label, ok := signalLabels[s]; ok {
			out = append(out, label)
		} else {
			out = append(out, s)
		}
	}
	return out
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

// formatFen renders an amount in fen as yuan with two decimals and
// thousands grouping, e.g. 219600 -> "¥2,196.00".
func formatFen(fen int64) string {
	sign := ""
	if fen < 0 {
		sign, fen = "-", -fen
	}
	cents := fen % 100
	return fmt.Sprintf("%s¥%s.%02d", sign, groupDigits(fmt.Sprintf("%d", fen/100)), cents)
}

func groupDigits(digits string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	for i, d := range []byte(digits) {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(d)
	}
	return b.String()
}

// renderTable pads cells to the widest cell per column. rightAlign marks
// columns whose cells are padded on the left instead.
func renderTable(header []string, rows [][]string, rightAlign map[int]bool) string {
	cols := len(header)
	w := make([]int, cols)
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
	pad := func(cell string, width int, right bool) string {
		gap := width - displayWidth(cell)
		if right {
			return strings.Repeat(" ", gap) + cell
		}
		return cell + strings.Repeat(" ", gap)
	}
	var b strings.Builder
	writeRow := func(row []string) {
		for i, cell := range row {
			b.WriteString(pad(cell, w[i], rightAlign[i]))
			if i < len(row)-1 {
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}
	writeRow(header)
	for _, row := range rows {
		writeRow(row)
	}
	return b.String()
}

// displayWidth returns the terminal cell width of s, counting East Asian
// wide runes as two columns so CJK text aligns.
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
