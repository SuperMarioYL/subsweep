package bills_test

import (
	"os"
	"testing"

	"github.com/SuperMarioYL/subsweep/internal/bills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shipped sample exports are the documented quickstart input: they must
// keep parsing, and keep producing a non-empty checklist, or the first-run
// experience breaks silently.
func TestParseShippedAlipaySample(t *testing.T) {
	raw, err := os.ReadFile("testdata/alipay_sample.csv")
	require.NoError(t, err)

	st, err := bills.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, bills.PlatformAlipay, st.Platform)
	assert.Len(t, st.Deductions, 27)

	sus := st.Suspects()
	assert.NotEmpty(t, sus)
	for _, s := range sus {
		assert.Equal(t, bills.DirectionExpense, s.Deduction.Direction,
			"income rows must never be flagged")
		assert.Greater(t, s.Confidence, 0.0)
		assert.LessOrEqual(t, s.Confidence, 1.0)
	}
	// Ordinary spend and the transfer income stay out of the checklist.
	for _, s := range sus {
		assert.NotEqual(t, "滴滴出行", s.Deduction.RawMerchant)
		assert.NotEqual(t, "王小明", s.Deduction.RawMerchant)
	}
}

func TestParseShippedWechatSample(t *testing.T) {
	raw, err := os.ReadFile("testdata/wechat_sample.csv")
	require.NoError(t, err)

	st, err := bills.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, bills.PlatformWechat, st.Platform)
	assert.Len(t, st.Deductions, 22)

	sus := st.Suspects()
	assert.NotEmpty(t, sus)
	for _, s := range sus {
		assert.Equal(t, bills.DirectionExpense, s.Deduction.Direction)
	}
	for _, s := range sus {
		assert.NotEqual(t, "瑞幸咖啡", s.Deduction.RawMerchant, "one-off spend is not a renewal candidate")
		assert.NotEqual(t, "微信红包", s.Deduction.Category, "income rows are never flagged")
	}
}
