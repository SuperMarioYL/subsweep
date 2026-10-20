// Package bills parses official Alipay and WeChat bill exports into
// structured deduction rows and flags probable auto-renewal charges for
// user confirmation.
package bills

// Platform identifies the app that produced a bill export.
type Platform string

const (
	PlatformAlipay Platform = "alipay"
	PlatformWechat Platform = "wechat"
)

// Names of the detection signals attached to a Suspect: the merchant hit
// the built-in renewal-service list, the same merchant and amount recurred
// monthly, or the category/remark fields mention automatic deduction.
const (
	SignalKnownMerchant = "known_merchant"
	SignalMonthlyRepeat = "monthly_repeat"
	SignalKeyword       = "keyword"
)

// Deduction is one transaction row from an official bill export.
type Deduction struct {
	Platform       Platform
	RowID          string
	Date           string // YYYY-MM-DD
	RawMerchant    string
	CounterAccount string
	AmountFen      int64
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

// Parse decodes and parses one official bill export, detecting the
// platform (Alipay exports are GB18030, WeChat exports UTF-8) and the
// format automatically. Unknown or empty input is an error.
func Parse(raw []byte) (*Statement, error) {
	return &Statement{}, nil // TODO: implement format detection and CSV parsing
}

// Suspects applies the renewal heuristics (known merchant list, same-amount
// monthly repetition, category/remark keywords) to the statement and
// returns the flagged rows for user confirmation.
func (s *Statement) Suspects() []Suspect {
	return nil // TODO: implement detection heuristics
}
