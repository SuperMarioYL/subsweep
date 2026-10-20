package report_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/SuperMarioYL/subsweep/internal/bills"
	"github.com/SuperMarioYL/subsweep/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// row builds one flagged deduction row. Signals are passed in a scrambled
// order on purpose so tests can pin the canonical output ordering.
func row(platform bills.Platform, rowID, date, merchant string, fen int64, confidence float64, signals ...string) bills.Suspect {
	return bills.Suspect{
		Deduction: bills.Deduction{
			Platform:    platform,
			RowID:       rowID,
			Date:        date,
			RawMerchant: merchant,
			AmountFen:   fen,
		},
		Signals:    signals,
		Confidence: confidence,
	}
}

func statement(platform bills.Platform, n int) *bills.Statement {
	st := &bills.Statement{Platform: platform}
	for i := 0; i < n; i++ {
		st.Deductions = append(st.Deductions, bills.Deduction{
			Platform: platform,
			RowID:    fmt.Sprintf("%s-row-%d", platform, i),
		})
	}
	return st
}

// alipayIqiyiSuspects mirrors the golden bill: three monthly rows of the
// same signing, the merchant recorded under its company name.
func alipayIqiyiSuspects() []bills.Suspect {
	return []bills.Suspect{
		row(bills.PlatformAlipay, "A1", "2024-06-10", "上海恩空中文化传播有限公司", 2500, 0.85, bills.SignalKeyword, bills.SignalMonthlyRepeat),
		row(bills.PlatformAlipay, "A2", "2024-07-10", "上海恩空中文化传播有限公司", 2500, 0.70, bills.SignalMonthlyRepeat),
		row(bills.PlatformAlipay, "A3", "2024-08-10", "上海恩空中文化传播有限公司", 2500, 0.70, bills.SignalMonthlyRepeat),
	}
}

func wechatTencentSuspects() []bills.Suspect {
	return []bills.Suspect{
		row(bills.PlatformWechat, "W1", "2024-06-12", "腾讯视频", 3000, 0.90, bills.SignalKeyword, bills.SignalKnownMerchant, bills.SignalMonthlyRepeat),
		row(bills.PlatformWechat, "W2", "2024-07-12", "腾讯视频", 3000, 0.80, bills.SignalKnownMerchant, bills.SignalMonthlyRepeat),
		row(bills.PlatformWechat, "W3", "2024-08-12", "腾讯视频", 3000, 0.80, bills.SignalKnownMerchant, bills.SignalMonthlyRepeat),
	}
}

var iqiyiService = report.Service{
	CanonicalName: "爱奇艺",
	Cancel: report.CancelPath{
		App:   "爱奇艺 App",
		Steps: []string{"我的", "会员", "自动续费管理", "关闭自动续费"},
		URL:   "https://www.iqiyi.com/vip/cancel",
	},
}

var tencentService = report.Service{
	CanonicalName: "腾讯视频",
	Cancel: report.CancelPath{
		App:   "腾讯视频 App",
		Steps: []string{"个人中心", "VIP会员", "自动续费管理", "关闭"},
		URL:   "https://film.qq.com/vip/cancel",
	},
}

type stubCatalog map[string]report.Service

func (c stubCatalog) Find(merchant string) (report.Service, bool) {
	s, ok := c[merchant]
	return s, ok
}

func catalogStub() stubCatalog {
	return stubCatalog{
		"上海恩空中文化传播有限公司": iqiyiService,
		"腾讯视频": tencentService,
	}
}

// fullInput is the shared scenario: three signings across two platforms,
// one of them (百度网盘) deliberately absent from the catalog.
func fullInput() report.Input {
	suspects := append(alipayIqiyiSuspects(), wechatTencentSuspects()...)
	suspects = append(suspects,
		row(bills.PlatformAlipay, "B1", "2024-09-15", "百度网盘", 12800, 0.60, bills.SignalKnownMerchant),
	)
	return report.Input{
		Statements: []*bills.Statement{statement(bills.PlatformAlipay, 6), statement(bills.PlatformWechat, 5)},
		Suspects:   suspects,
		Catalog:    catalogStub(),
		Now:        now,
	}
}

func TestBuildGroupsRowsIntoSigningsAndAnnualizes(t *testing.T) {
	r := report.Build(fullInput())

	require.Len(t, r.Items, 3)
	assert.Equal(t, 2, r.Statements)
	assert.Equal(t, 11, r.Transactions)

	// Ordered by annualized cost, biggest first.
	cloud, tencent, iqiyi := r.Items[0], r.Items[1], r.Items[2]

	assert.Equal(t, "百度网盘", cloud.Merchant)
	assert.Equal(t, int64(12800), cloud.AmountFen)
	assert.Equal(t, int64(153600), cloud.AnnualizedFen)
	assert.Equal(t, 1, cloud.Charges)
	assert.Nil(t, cloud.Service)

	assert.Equal(t, "腾讯视频", tencent.Merchant)
	assert.Equal(t, bills.PlatformWechat, tencent.Platform)
	assert.Equal(t, int64(36000), tencent.AnnualizedFen)
	assert.Equal(t, 3, tencent.Charges)
	assert.Equal(t, "2024-06-12", tencent.FirstDate)
	assert.Equal(t, "2024-08-12", tencent.LastDate)
	assert.Equal(t, 0.90, tencent.Confidence, "item confidence is the max across rows")
	assert.Equal(t, []string{
		bills.SignalKnownMerchant, bills.SignalMonthlyRepeat, bills.SignalKeyword,
	}, tencent.Signals, "signals are deduplicated in canonical order regardless of input order")
	require.NotNil(t, tencent.Service)
	assert.Equal(t, "腾讯视频", tencent.Service.CanonicalName)

	// The catalog maps the company name to the canonical service, so the
	// checklist shows 爱奇艺 while keeping the bill's original string.
	assert.Equal(t, "爱奇艺", iqiyi.Merchant)
	assert.Equal(t, "上海恩空中文化传播有限公司", iqiyi.RawMerchant)
	assert.Equal(t, bills.PlatformAlipay, iqiyi.Platform)
	assert.Equal(t, int64(30000), iqiyi.AnnualizedFen)

	assert.Equal(t, int64(219600), r.AnnualizedFen, "total = 153600 + 36000 + 30000")

	charges := 0
	for _, it := range r.Items {
		charges += it.Charges
	}
	assert.Equal(t, 7, charges)

	require.Len(t, r.Forgotten, 3, "nothing is confirmed yet, so every signing is a suspected forgotten one")
}

func TestBuildCountsAnnualizedTotalPerSigningNotPerRow(t *testing.T) {
	r := report.Build(report.Input{Suspects: alipayIqiyiSuspects(), Now: now})

	require.Len(t, r.Items, 1)
	assert.Equal(t, 3, r.Items[0].Charges)
	assert.Equal(t, int64(30000), r.AnnualizedFen,
		"three monthly rows of one signing annualize once (25×12), not 3×25×12")
}

func TestBuildIgnoresDuplicateRows(t *testing.T) {
	suspects := alipayIqiyiSuspects()
	suspects = append(suspects, suspects[0])

	r := report.Build(report.Input{Suspects: suspects, Now: now})

	require.Len(t, r.Items, 1)
	assert.Equal(t, 3, r.Items[0].Charges)
	assert.Equal(t, int64(30000), r.AnnualizedFen)
}

func TestBuildStatusesAndForgottenOrdering(t *testing.T) {
	suspects := append(alipayIqiyiSuspects(), wechatTencentSuspects()...)
	suspects = append(suspects,
		row(bills.PlatformWechat, "W9", "2024-09-01", "网易云音乐", 1500, 0.55, bills.SignalKeyword),
	)

	r := report.Build(report.Input{
		Suspects: suspects,
		Catalog:  catalogStub(),
		Statuses: map[string]report.Status{
			"A1": report.StatusConfirmed,
			"A2": report.StatusConfirmed,
			"A3": report.StatusConfirmed,
			"W2": report.StatusUnrecognized,
			// W1, W3 stay pending; the 网易云音乐 row stays pending too.
		},
		Now: now,
	})

	require.Len(t, r.Items, 3)
	byMerchant := map[string]*report.Item{}
	for _, it := range r.Items {
		byMerchant[it.Merchant] = it
	}
	assert.Equal(t, report.StatusConfirmed, byMerchant["爱奇艺"].Status)
	assert.Equal(t, report.StatusUnrecognized, byMerchant["腾讯视频"].Status)
	assert.Equal(t, report.StatusPending, byMerchant["网易云音乐"].Status)

	require.Len(t, r.Forgotten, 2)
	assert.Equal(t, "腾讯视频", r.Forgotten[0].Merchant, "unrecognized signings rank ahead of pending ones")
	assert.Equal(t, "网易云音乐", r.Forgotten[1].Merchant)
}

func TestBuildSeriesStatusIsWorstOfItsRows(t *testing.T) {
	r := report.Build(report.Input{
		Suspects: alipayIqiyiSuspects(),
		Statuses: map[string]report.Status{
			"A1": report.StatusConfirmed,
			"A2": report.StatusUnrecognized,
			// A3 stays pending.
		},
		Now: now,
	})

	require.Len(t, r.Items, 1)
	assert.Equal(t, report.StatusUnrecognized, r.Items[0].Status,
		"one unrecognized row marks the whole signing unrecognized")
}

func TestBuildWithoutCatalogLeavesServiceUnknown(t *testing.T) {
	r := report.Build(report.Input{Suspects: alipayIqiyiSuspects(), Now: now})

	require.Len(t, r.Items, 1)
	assert.Nil(t, r.Items[0].Service)
	assert.Equal(t, "上海恩空中文化传播有限公司", r.Items[0].Merchant)
	assert.Equal(t, "上海恩空中文化传播有限公司", r.Items[0].RawMerchant)
}

func TestBuildEmptyInput(t *testing.T) {
	r := report.Build(report.Input{Now: now})

	assert.Empty(t, r.Items)
	assert.Empty(t, r.Forgotten)
	assert.Equal(t, int64(0), r.AnnualizedFen)
	assert.Equal(t, now, r.GeneratedAt)

	assert.Contains(t, r.Markdown(), "未检出疑似自动续费扣款")
	assert.Contains(t, r.Table(), "未检出疑似自动续费扣款")

	data, err := r.JSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"items": []`)
}

func TestBuildFallsBackToWallClockWhenNowIsZero(t *testing.T) {
	before := time.Now()
	r := report.Build(report.Input{})
	assert.False(t, r.GeneratedAt.IsZero())
	assert.WithinDuration(t, before, r.GeneratedAt, time.Minute)
}

func TestMarkdownReport(t *testing.T) {
	r := report.Build(fullInput())
	md := r.Markdown()

	assert.Contains(t, md, "# 续费体检报告")
	assert.Contains(t, md, "生成时间：2026-10-21 12:00")
	assert.Contains(t, md, "扫描范围：2 份账单、11 笔交易；检出疑似自动续费 3 项（7 笔扣款）。")
	assert.Contains(t, md, "年化成本合计：**¥2,196.00**")

	assert.Contains(t, md, "## 疑似遗忘项")
	assert.Contains(t, md, "1. **百度网盘**（支付宝）— 月费 ¥128.00，年化 ¥1,536.00，状态：待确认")

	assert.Contains(t, md, "### 爱奇艺（支付宝）")
	assert.Contains(t, md, "账单商户：上海恩空中文化传播有限公司")
	assert.Contains(t, md, "扣款 3 笔（2024-06-10 至 2024-08-10）")
	assert.Contains(t, md, "命中信号：按月重复扣款、备注含代扣字样")
	assert.Contains(t, md, "置信度 0.85 ｜ 状态：待确认")
	assert.Contains(t, md, "官方取消路径：爱奇艺 App：我的 → 会员 → 自动续费管理 → 关闭自动续费")
	assert.Contains(t, md, "在线入口：https://www.iqiyi.com/vip/cancel")

	assert.Contains(t, md, "官方取消路径：知识库暂未收录该商户", "unmatched merchants point at the app instead of a fabricated path")
	assert.Contains(t, md, "## 说明")
}

func TestTableReport(t *testing.T) {
	r := report.Build(fullInput())
	table := r.Table()

	assert.Contains(t, table, "续费体检清单（2026-10-21 12:00）")
	assert.Contains(t, table, "年化合计 ¥2,196.00")
	assert.Contains(t, table, "已确认 0 项、待确认 3 项、不认识 0 项")

	for _, want := range []string{"商户", "平台", "月费", "年化", "信号", "置信度", "状态", "取消入口"} {
		assert.Contains(t, table, want, "table header should list %s", want)
	}
	assert.Contains(t, table, "百度网盘")
	assert.Contains(t, table, "未收录", "unknown merchants show up as missing from the catalog")
	assert.Contains(t, table, "待核实 3 项")
}

func TestJSONReport(t *testing.T) {
	r := report.Build(fullInput())
	data, err := r.JSON()
	require.NoError(t, err)

	var doc struct {
		Schema        string    `json:"schema"`
		GeneratedAt   time.Time `json:"generated_at"`
		Statements    int       `json:"statements"`
		Transactions  int       `json:"transactions"`
		AnnualizedFen int64     `json:"annualized_total_fen"`
		Items         []struct {
			Platform      string   `json:"platform"`
			Merchant      string   `json:"merchant"`
			RawMerchant   string   `json:"raw_merchant"`
			AmountFen     int64    `json:"amount_fen"`
			AnnualizedFen int64    `json:"annualized_fen"`
			FirstDate     string   `json:"first_date"`
			LastDate      string   `json:"last_date"`
			Charges       int      `json:"charges"`
			Signals       []string `json:"signals"`
			Confidence    float64  `json:"confidence"`
			Status        string   `json:"status"`
			Service       *struct {
				CanonicalName string `json:"canonical_name"`
				Cancel        struct {
					App   string   `json:"app"`
					Steps []string `json:"steps"`
					URL   string   `json:"url"`
				} `json:"cancel"`
			} `json:"service"`
			RowIDs []string `json:"row_ids"`
		} `json:"items"`
		Forgotten []json.RawMessage `json:"forgotten"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "subsweep.report.v1", doc.Schema)
	assert.Equal(t, now, doc.GeneratedAt)
	assert.Equal(t, 2, doc.Statements)
	assert.Equal(t, 11, doc.Transactions)
	assert.Equal(t, int64(219600), doc.AnnualizedFen)

	require.Len(t, doc.Items, 3)
	cloud, tencent, iqiyi := doc.Items[0], doc.Items[1], doc.Items[2]

	assert.Equal(t, "alipay", cloud.Platform)
	assert.Equal(t, "百度网盘", cloud.Merchant)
	assert.Nil(t, cloud.Service)

	assert.Equal(t, "wechat", tencent.Platform)
	assert.Equal(t, "pending", tencent.Status)
	assert.Equal(t, []string{"known_merchant", "monthly_repeat", "keyword"}, tencent.Signals)
	require.Len(t, tencent.RowIDs, 3)
	assert.Equal(t, []string{"W1", "W2", "W3"}, tencent.RowIDs)

	require.NotNil(t, iqiyi.Service)
	assert.Equal(t, "爱奇艺", iqiyi.Service.CanonicalName)
	assert.Equal(t, []string{"我的", "会员", "自动续费管理", "关闭自动续费"}, iqiyi.Service.Cancel.Steps)
	assert.Equal(t, "https://www.iqiyi.com/vip/cancel", iqiyi.Service.Cancel.URL)

	assert.Len(t, doc.Forgotten, 3)
}
