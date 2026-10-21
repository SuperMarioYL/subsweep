// Package bills parses official Alipay and WeChat bill exports into
// structured deduction rows and flags probable auto-renewal charges for
// user confirmation.
package bills

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SuperMarioYL/subsweep/internal/catalog"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// Platform identifies the app that produced a bill export.
type Platform string

const (
	PlatformAlipay Platform = "alipay"
	PlatformWechat Platform = "wechat"
)

// Direction separates money out from money in. Detection heuristics only
// ever consider expenses; income rows (red packets, transfers) are kept in
// the statement for completeness but can never be flagged.
type Direction string

const (
	DirectionExpense Direction = "expense"
	DirectionIncome  Direction = "income"
	DirectionNeutral Direction = "neutral"
)

// Names of the detection signals attached to a Suspect: the merchant hit
// the built-in renewal-service list, the same merchant and amount recurred
// monthly, or the category/remark fields mention automatic deduction.
const (
	SignalKnownMerchant = "known_merchant"
	SignalMonthlyRepeat = "monthly_repeat"
	SignalKeyword       = "keyword"
)

// renewalKeywords are the strings that mark a category/remark field as an
// automatic-deduction mention. Only 交易分类/交易类型 and 备注 are scanned:
// the product description column routinely says things like "连续包月" on
// rows that are plain product text, so it is deliberately excluded.
var renewalKeywords = []string{"代扣", "自动续费", "自动扣款", "自动续订", "周期扣款"}

// Confidence weights per signal. Hitting the curated service list is the
// strongest evidence; monthly repetition and explicit deduction wording
// add on top. The sum is capped below 1.0 because detection is heuristic —
// only the user's confirmation makes it certain.
const (
	weightKnownMerchant = 0.55
	weightMonthlyRepeat = 0.30
	weightKeyword       = 0.15
	maxConfidence       = 0.95
)

// Deduction is one transaction row from an official bill export.
type Deduction struct {
	Platform       Platform
	RowID          string
	Date           string // YYYY-MM-DD
	RawMerchant    string
	CounterAccount string
	AmountFen      int64
	Direction      Direction
	Category       string // 交易分类 (Alipay) / 交易类型 (WeChat)
	Description    string // 商品说明 (Alipay) / 商品 (WeChat)
	Remark         string // 备注
}

// Statement is the parsed content of one bill export.
type Statement struct {
	Platform   Platform
	Deductions []Deduction
}

// Suspect is a deduction flagged as a probable auto-renewal charge,
// pending user confirmation. Signals lists the heuristics that fired.
type Suspect struct {
	Deduction  Deduction
	Signals    []string
	Confidence float64 // 0..1
}

// rowSpec maps one platform's header columns onto Deduction fields.
// startCols index from the row's beginning; endCols index backwards from
// the row's end (-1 = last field). WeChat needs both anchors: real exports
// carry an extra unnamed empty column on merchant rows, so tail columns
// (IDs, remark, amount…) stay aligned relative to the row end while the
// head columns stay fixed.
type rowSpec struct {
	platform  Platform
	fields    []string
	startCols map[string]int
	endCols   map[string]int
}

var alipaySpec = rowSpec{
	platform: PlatformAlipay,
	fields: []string{
		"交易时间", "交易分类", "交易对方", "对方账号", "商品说明", "收/支",
		"金额", "收/付款方式", "交易状态", "交易订单号", "商家订单号", "备注",
	},
	startCols: map[string]int{
		"交易时间": 0, "交易分类": 1, "交易对方": 2, "商品说明": 4, "收/支": 5,
		"金额": 6, "收/付款方式": 7, "交易订单号": 9, "商家订单号": 10, "备注": 11,
	},
}

var wechatSpec = rowSpec{
	platform: PlatformWechat,
	fields: []string{
		"交易时间", "交易类型", "交易对方", "商品", "收/支",
		"金额(元)", "支付方式", "当前状态", "交易单号", "商户单号", "备注",
	},
	startCols: map[string]int{
		"交易时间": 0, "交易类型": 1, "交易对方": 2,
	},
	endCols: map[string]int{
		"商品": -8, "收/支": -7, "金额(元)": -6, "支付方式": -5,
		"当前状态": -4, "交易单号": -3, "商户单号": -2, "备注": -1,
	},
}

// specFor picks the platform's column layout from the header row. Alipay
// and WeChat share the leading 交易时间 column, so the distinguishing fields
// are the amount and payment-method columns.
func specFor(header []string) (rowSpec, error) {
	index := map[string]int{}
	for i, name := range header {
		index[strings.TrimSpace(name)] = i
	}
	matches := func(spec rowSpec) bool {
		for _, name := range spec.fields {
			if _, ok := index[name]; !ok {
				return false
			}
		}
		return true
	}
	switch {
	case matches(wechatSpec):
		// The header must match the canonical column order exactly.
		for i, name := range wechatSpec.fields {
			if index[name] != i {
				return rowSpec{}, fmt.Errorf("bills: 微信账单表头列 %q 位置异常（%d ≠ %d）", name, index[name], i)
			}
		}
		return wechatSpec, nil
	case matches(alipaySpec):
		for i, name := range alipaySpec.fields {
			if index[name] != i {
				return rowSpec{}, fmt.Errorf("bills: 支付宝账单表头列 %q 位置异常（%d ≠ %d）", name, index[name], i)
			}
		}
		return alipaySpec, nil
	default:
		return rowSpec{}, errors.New("bills: 无法识别账单格式：既不是支付宝也不是微信的官方导出")
	}
}

// Parse decodes and parses one official bill export, detecting the
// platform (Alipay exports are GB18030, WeChat exports UTF-8) and the
// format automatically. Unknown or empty input is an error.
func Parse(raw []byte) (*Statement, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("bills: 输入为空")
	}

	text, err := decode(raw)
	if err != nil {
		return nil, err
	}

	headerIdx, header := findHeader(splitLines(text))
	if headerIdx < 0 {
		return nil, errors.New("bills: 未找到官方导出的表头行（交易时间…），无法识别账单格式")
	}
	spec, err := specFor(header)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(strings.Join(linesFrom(text, headerIdx), "\n")))
	reader.FieldsPerRecord = -1 // real exports pad rows with trailing commas

	st := &Statement{Platform: spec.platform}
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("bills: 解析 CSV 行失败: %w", err)
		}
		if len(rec) == 0 || strings.TrimSpace(rec[0]) == "" || rec[0] == "交易时间" {
			continue // blank line or a repeated page header
		}
		d, err := spec.parseRow(rec)
		if err != nil {
			return nil, err
		}
		st.Deductions = append(st.Deductions, d)
	}
	return st, nil
}

// decode detects the encoding: valid UTF-8 (including the BOM WeChat adds)
// passes through, everything else is treated as the GB18030 Alipay uses.
func decode(raw []byte) (string, error) {
	if utf8.Valid(raw) {
		return strings.TrimPrefix(string(raw), "\ufeff"), nil
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
	if err != nil {
		return "", fmt.Errorf("bills: 账单既不是 UTF-8 也无法按 GB18030 解码: %w", err)
	}
	return strings.TrimPrefix(string(decoded), "\ufeff"), nil
}

func splitLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func linesFrom(text string, from int) []string {
	lines := splitLines(text)
	if from >= len(lines) {
		return nil
	}
	return lines[from:]
}

// findHeader locates the first line whose first CSV field is 交易时间 —
// both exports carry a metadata preamble above it.
func findHeader(lines []string) (int, []string) {
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "交易时间") {
			continue
		}
		rec, err := csv.NewReader(strings.NewReader(line)).Read()
		if err != nil {
			continue
		}
		return i, rec
	}
	return -1, nil
}

// parseRow maps one CSV record onto a Deduction using the platform layout.
func (sp rowSpec) parseRow(rec []string) (Deduction, error) {
	get := func(name string) string {
		if i, ok := sp.startCols[name]; ok {
			if i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		if off, ok := sp.endCols[name]; ok {
			i := len(rec) + off
			if i < 0 || i >= len(rec) {
				return ""
			}
			// The end-anchored section must not overlap the fixed head.
			if i <= maxStartIndex(sp) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	date, err := parseDate(get("交易时间"))
	if err != nil {
		return Deduction{}, err
	}
	amountCol := "金额"
	if _, ok := sp.endCols["金额(元)"]; ok {
		amountCol = "金额(元)"
	}
	fen, err := parseFen(get(amountCol))
	if err != nil {
		return Deduction{}, fmt.Errorf("bills: 解析金额 %q 失败: %w", get(amountCol), err)
	}

	d := Deduction{
		Platform:       sp.platform,
		Date:           date,
		RawMerchant:    get("交易对方"),
		CounterAccount: get("收/付款方式"),
		AmountFen:      fen,
		Direction:      directionOf(get("收/支")),
		Category:       get("交易分类"),
		Description:    get("商品说明"),
		Remark:         get("备注"),
	}
	if d.CounterAccount == "" {
		d.CounterAccount = get("支付方式")
	}
	if d.Category == "" {
		d.Category = get("交易类型")
	}
	if d.Description == "" {
		d.Description = get("商品")
	}
	d.RowID = get("交易订单号")
	if d.RowID == "" {
		d.RowID = get("商家订单号")
	}
	if d.RowID == "" {
		d.RowID = get("交易单号")
	}
	if d.RowID == "" {
		d.RowID = get("商户单号")
	}
	if d.RowID == "" {
		d.RowID = fmt.Sprintf("%s|%s|%d", date, d.RawMerchant, fen)
	}
	return d, nil
}

// maxStartIndex is the last fixed head column; end-anchored lookups may not
// reach into it even on short rows.
func maxStartIndex(sp rowSpec) int {
	max := -1
	for _, i := range sp.startCols {
		if i > max {
			max = i
		}
	}
	return max
}

// Suspects applies the renewal heuristics to the statement and returns the
// flagged rows for user confirmation — a checklist to review, never a
// mechanical verdict. A row is flagged when any of these fire:
//
//  1. the merchant name hits the embedded renewal-service list
//     (canonical name, billing alias or distinctive keyword);
//  2. the same merchant charged the same amount on a monthly cadence;
//  3. the category/remark fields mention 代扣/自动续费-style wording.
//
// Income rows are never flagged.
func (s *Statement) Suspects() []Suspect {
	if s == nil || len(s.Deductions) == 0 {
		return nil
	}
	kb := catalog.Default()

	// Group expense rows by (merchant, amount) to find monthly series.
	type seriesKey struct {
		merchant string
		fen      int64
	}
	series := map[seriesKey][]*Deduction{}
	for i := range s.Deductions {
		d := &s.Deductions[i]
		if d.Direction != DirectionExpense {
			continue
		}
		k := seriesKey{d.RawMerchant, d.AmountFen}
		series[k] = append(series[k], d)
	}
	repeated := map[*Deduction]bool{}
	for _, rows := range series {
		if hasMonthlyCadence(rows) {
			for _, d := range rows {
				repeated[d] = true
			}
		}
	}

	var out []Suspect
	for i := range s.Deductions {
		d := &s.Deductions[i]
		if d.Direction != DirectionExpense {
			continue
		}
		var signals []string
		if _, ok := kb.Find(d.RawMerchant); ok {
			signals = append(signals, SignalKnownMerchant)
		}
		if repeated[d] {
			signals = append(signals, SignalMonthlyRepeat)
		}
		if hasRenewalKeyword(d.Category) || hasRenewalKeyword(d.Remark) {
			signals = append(signals, SignalKeyword)
		}
		if len(signals) == 0 {
			continue
		}
		out = append(out, Suspect{
			Deduction:  *d,
			Signals:    signals,
			Confidence: confidence(signals),
		})
	}
	return out
}

// hasMonthlyCadence reports whether a same-amount series repeats on a
// monthly rhythm: some consecutive pair sits 27–33 days apart, which
// tolerates short and long months while ignoring coincidental one-offs.
func hasMonthlyCadence(rows []*Deduction) bool {
	if len(rows) < 2 {
		return false
	}
	dates := make([]time.Time, 0, len(rows))
	for _, d := range rows {
		t, err := time.Parse("2006-01-02", d.Date)
		if err != nil {
			return false
		}
		dates = append(dates, t)
	}
	for i := 1; i < len(dates); i++ {
		gap := int(dates[i].Sub(dates[i-1]).Hours() / 24)
		if gap >= 27 && gap <= 33 {
			return true
		}
	}
	return false
}

func hasRenewalKeyword(field string) bool {
	if field == "" {
		return false
	}
	for _, kw := range renewalKeywords {
		if strings.Contains(field, kw) {
			return true
		}
	}
	return false
}

func confidence(signals []string) float64 {
	score := 0.0
	for _, sig := range signals {
		switch sig {
		case SignalKnownMerchant:
			score += weightKnownMerchant
		case SignalMonthlyRepeat:
			score += weightMonthlyRepeat
		case SignalKeyword:
			score += weightKeyword
		}
	}
	if score > maxConfidence {
		score = maxConfidence
	}
	return score
}

func directionOf(s string) Direction {
	switch s {
	case "支出":
		return DirectionExpense
	case "收入":
		return DirectionIncome
	default:
		return DirectionNeutral
	}
}

// parseDate keeps the YYYY-MM-DD part of a 交易时间 cell.
func parseDate(s string) (string, error) {
	if len(s) < 10 {
		return "", fmt.Errorf("bills: 无法解析交易时间 %q", s)
	}
	date := s[:10]
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", fmt.Errorf("bills: 无法解析交易时间 %q", s)
	}
	return date, nil
}

// parseFen converts an amount cell ("18.60", "¥30.00", "1,236.5") into
// integer fen without going through float64.
func parseFen(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "¥", "")
	s = strings.ReplaceAll(s, "￥", "")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, errors.New("empty amount")
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	intPart, fracPart := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > 2 {
		fracPart = fracPart[:2]
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	for _, c := range intPart + fracPart {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("非数字金额 %q", s)
		}
	}
	yuan := 0
	for _, c := range intPart {
		yuan = yuan*10 + int(c-'0')
	}
	fen := int64(yuan)*100 + int64(fracPart[0]-'0')*10 + int64(fracPart[1]-'0')
	if neg {
		fen = -fen
	}
	return fen, nil
}
