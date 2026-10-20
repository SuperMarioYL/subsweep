package bills_test

import (
	"testing"

	"github.com/SuperMarioYL/subsweep/internal/bills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// Synthetic golden samples modeled on the official export shapes:
// Alipay writes GB18030 with a metadata preamble and a 交易时间 header row;
// WeChat writes UTF-8 with a metadata preamble and quoted ¥ amounts.
const alipayFixture = `支付宝交易明细
账号:[181****5678]
起始日期:[2024-06-01 00:00:00],终止日期:[2024-09-30 23:59:59]
----------------------------------------[交易记录明细列表]----------------------------------------
交易时间,交易分类,交易对方,对方账号,商品说明,收/支,金额,收/付款方式,交易状态,交易订单号,商家订单号,备注
2024-06-03 08:11:32,交通出行,滴滴出行,--,快车-市区,支出,18.60,中国工商银行(0521),交易成功,2024060322001412341234567890,2024060322001412341234567890,--
2024-06-05 21:44:09,餐饮美食,美团平台商户,--,外卖订单,支出,35.50,中国工商银行(0521),交易成功,2024060522001412341234567891,2024060522001412341234567891,--
2024-06-10 10:02:11,休闲娱乐,上海恩空中文化传播有限公司,--,爱奇艺会员连续包月,支出,25.00,余额宝,交易成功,2024061022001412341234567892,2024061022001412341234567892,自动续费
2024-07-10 10:05:43,休闲娱乐,上海恩空中文化传播有限公司,--,爱奇艺会员连续包月,支出,25.00,余额宝,交易成功,2024071022001412341234567893,2024071022001412341234567893,--
2024-08-10 10:01:57,休闲娱乐,上海恩空中文化传播有限公司,--,爱奇艺会员连续包月,支出,25.00,余额宝,交易成功,2024081022001412341234567894,2024081022001412341234567894,--
2024-06-15 09:00:00,日用百货,杭州娃哈哈电子商务有限公司,--,农夫山泉饮用纯净水,支出,32.00,中国工商银行(0521),交易成功,2024061522001412341234567895,2024061522001412341234567895,--
`

const wechatFixture = `微信支付账单明细
起始时间:[2024-06-01 00:00:00] 终止时间:[2024-09-30 23:59:59]
导出类型:[全部]
导出时间:[2024-10-08 12:00:00]
共 5 笔记录
收入:1笔 支出:4笔 中性交易:0笔
备注:[以下是微信支付账单明细列表]
----------------------微信支付账单明细列表------------------------
交易时间,交易类型,交易对方,商品,收/支,金额(元),支付方式,当前状态,交易单号,商户单号,备注
2024-06-12 09:30:00,商户消费,腾讯视频,,VIP会员连续包月,支出,"¥30.00",零钱,支付成功,1000120240612093000112299001122,1000120240612000112299001122,自动续费
2024-07-12 09:30:05,商户消费,腾讯视频,,VIP会员连续包月,支出,"¥30.00",零钱,支付成功,1000120240712093000112299002122,1000120240712000212299002122,
2024-08-12 09:30:11,商户消费,腾讯视频,,VIP会员连续包月,支出,"¥30.00",零钱,支付成功,1000120240812093000112299003122,1000120240812000312299003122,
2024-06-18 18:22:40,商户消费,瑞幸咖啡,生椰拿铁,支出,"¥9.90",招商银行(5678),支付成功,1000120240618182200112299004122,1000120240618000412299004122,
2024-06-20 12:00:00,微信红包,,微信红包,收入,"¥88.88",零钱,已存入零钱,1000120240620120000112299005122,1000120240620000512299005122,
`

func mustGB18030(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(s))
	require.NoError(t, err)
	return raw
}

func findDeduction(t *testing.T, st *bills.Statement, rowID string) bills.Deduction {
	t.Helper()
	for _, d := range st.Deductions {
		if d.RowID == rowID {
			return d
		}
	}
	t.Fatalf("deduction with row id %q not found", rowID)
	return bills.Deduction{}
}

func suspectsByMerchant(sus []bills.Suspect, merchant string) []bills.Suspect {
	var out []bills.Suspect
	for _, s := range sus {
		if s.Deduction.RawMerchant == merchant {
			out = append(out, s)
		}
	}
	return out
}

func hasSignal(s bills.Suspect, signal string) bool {
	for _, sig := range s.Signals {
		if sig == signal {
			return true
		}
	}
	return false
}

func assertPlausibleConfidence(t *testing.T, sus []bills.Suspect) {
	t.Helper()
	for _, s := range sus {
		assert.Greater(t, s.Confidence, 0.0, "confidence must be positive for %q", s.Deduction.RawMerchant)
		assert.LessOrEqual(t, s.Confidence, 1.0, "confidence must not exceed 1 for %q", s.Deduction.RawMerchant)
	}
}

func TestParseAlipayGB18030(t *testing.T) {
	st, err := bills.Parse(mustGB18030(t, alipayFixture))
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, bills.PlatformAlipay, st.Platform)
	require.Len(t, st.Deductions, 6)

	ride := findDeduction(t, st, "2024060322001412341234567890")
	assert.Equal(t, bills.PlatformAlipay, ride.Platform)
	assert.Equal(t, "2024-06-03", ride.Date)
	assert.Equal(t, "滴滴出行", ride.RawMerchant)
	assert.Equal(t, "中国工商银行(0521)", ride.CounterAccount)
	assert.Equal(t, int64(1860), ride.AmountFen)

	renewal := findDeduction(t, st, "2024061022001412341234567892")
	assert.Equal(t, "2024-06-10", renewal.Date)
	assert.Equal(t, "上海恩空中文化传播有限公司", renewal.RawMerchant)
	assert.Equal(t, "余额宝", renewal.CounterAccount)
	assert.Equal(t, int64(2500), renewal.AmountFen)
}

func TestParseWechatUTF8(t *testing.T) {
	st, err := bills.Parse([]byte(wechatFixture))
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, bills.PlatformWechat, st.Platform)

	renewal := findDeduction(t, st, "1000120240612093000112299001122")
	assert.Equal(t, bills.PlatformWechat, renewal.Platform)
	assert.Equal(t, "2024-06-12", renewal.Date)
	assert.Equal(t, "腾讯视频", renewal.RawMerchant)
	assert.Equal(t, "零钱", renewal.CounterAccount)
	assert.Equal(t, int64(3000), renewal.AmountFen)

	snack := findDeduction(t, st, "1000120240618182200112299004122")
	assert.Equal(t, "瑞幸咖啡", snack.RawMerchant)
	assert.Equal(t, int64(990), snack.AmountFen)
}

func TestParseRejectsUnknownFormat(t *testing.T) {
	_, err := bills.Parse([]byte("name,amount\nlatte,4.50\n"))
	assert.Error(t, err, "a CSV matching neither export format must be rejected")
}

func TestParseRejectsEmptyInput(t *testing.T) {
	_, err := bills.Parse(nil)
	assert.Error(t, err)
}

func TestSuspectsAlipayMonthlyRepeatAndKeyword(t *testing.T) {
	st, err := bills.Parse(mustGB18030(t, alipayFixture))
	require.NoError(t, err)

	sus := st.Suspects()
	renewals := suspectsByMerchant(sus, "上海恩空中文化传播有限公司")
	require.Len(t, renewals, 3, "all three monthly occurrences should be flagged")

	keywordRows := 0
	for _, s := range renewals {
		assert.Contains(t, s.Signals, bills.SignalMonthlyRepeat)
		if hasSignal(s, bills.SignalKeyword) {
			keywordRows++
		}
	}
	assert.Equal(t, 1, keywordRows, "only the row whose remark mentions auto-renewal carries the keyword signal")
	assertPlausibleConfidence(t, renewals)

	// Ordinary spend is not a renewal candidate: the flagged set is exactly
	// the repeating series, nothing else.
	assert.Len(t, sus, 3)
}

func TestSuspectsWechatKnownMerchantMonthlyRepeatAndKeyword(t *testing.T) {
	st, err := bills.Parse([]byte(wechatFixture))
	require.NoError(t, err)

	sus := st.Suspects()
	renewals := suspectsByMerchant(sus, "腾讯视频")
	require.Len(t, renewals, 3, "all three monthly occurrences should be flagged")
	for _, s := range renewals {
		assert.Contains(t, s.Signals, bills.SignalMonthlyRepeat)
		assert.Contains(t, s.Signals, bills.SignalKnownMerchant)
	}
	assertPlausibleConfidence(t, renewals)

	var keywordRows int
	for _, s := range renewals {
		if hasSignal(s, bills.SignalKeyword) {
			keywordRows++
			assert.Equal(t, "2024-06-12", s.Deduction.Date, "only the June row's remark mentions auto-renewal")
		}
	}
	assert.Equal(t, 1, keywordRows, "only the row whose remark mentions auto-renewal carries the keyword signal")

	// One-off spend from an unlisted merchant and income rows stay out.
	assert.Len(t, suspectsByMerchant(sus, "瑞幸咖啡"), 0)
	assert.Len(t, sus, 3)
	for _, s := range sus {
		assert.NotEqual(t, "1000120240620120000112299005122", s.Deduction.RowID, "income rows must never be flagged")
	}
}

func TestSuspectsOnEmptyStatement(t *testing.T) {
	st := &bills.Statement{}
	assert.Empty(t, st.Suspects())
}
