package catalog_test

import (
	"strings"
	"testing"

	"github.com/SuperMarioYL/subsweep/internal/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The knowledge base is the product's moat: it must stay complete, every
// entry must carry a real cancel path, and lookups must be unambiguous.
func TestLoadEmbeddedKB(t *testing.T) {
	kb, err := catalog.Load()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, kb.Size(), 50, "the plan promises 50 curated services")

	for _, svc := range kb.Services() {
		assert.NotEmpty(t, svc.CanonicalName)
		assert.NotEmpty(t, svc.Cancel.App, "service %q must name its cancel app", svc.CanonicalName)
		assert.GreaterOrEqual(t, len(svc.Cancel.Steps), 2, "service %q needs a step-by-step path", svc.CanonicalName)
		assert.True(t, strings.HasPrefix(svc.Cancel.URL, "https://"),
			"service %q must link an official https page", svc.CanonicalName)
	}
}

func TestFindCanonicalAliasAndKeyword(t *testing.T) {
	kb, err := catalog.Load()
	require.NoError(t, err)

	cases := []struct {
		merchant string
		want     string
	}{
		{"腾讯视频", "腾讯视频"},                 // canonical name
		{"上海恩空中文化传播有限公司", "爱奇艺"},         // billing company alias
		{"北京百度网讯科技有限公司", "百度网盘"},         // billing company alias
		{"Apple Distribute.", "iCloud+"}, // punctuation and case are normalized away
		{"爱奇艺黄金VIP会员连续包月", "爱奇艺"},        // distinctive keyword
		{"网易云音乐黑胶VIP连续包月", "网易云音乐"},      // distinctive keyword
	}
	for _, tc := range cases {
		svc, ok := kb.Find(tc.merchant)
		require.True(t, ok, "Find(%q) should match", tc.merchant)
		assert.Equal(t, tc.want, svc.CanonicalName)
	}
}

// Ordinary merchants must never resolve to a renewal service — a false
// match would put a wrong cancel path on an unrelated charge.
func TestFindRejectsOrdinaryMerchants(t *testing.T) {
	kb, err := catalog.Load()
	require.NoError(t, err)

	for _, merchant := range []string{
		"", "滴滴出行", "美团平台商户", "瑞幸咖啡", "农夫山泉", "星巴克", "杭州优卖网络科技有限公司",
	} {
		_, ok := kb.Find(merchant)
		assert.False(t, ok, "Find(%q) must not match", merchant)
	}
}

func TestParseRejectsBrokenKnowledge(t *testing.T) {
	_, err := catalog.Parse([]byte(`{"version":1,"services":[{"canonical_name":"","cancel":{"app":"x","steps":["a","b"],"url":"https://a.b"}}]}`))
	assert.Error(t, err)

	_, err = catalog.Parse([]byte(`{"version":1,"services":[{"canonical_name":"A","cancel":{"app":"x","steps":["a","b"],"url":"https://a.b"}},{"canonical_name":"A","cancel":{"app":"x","steps":["a","b"],"url":"https://a.b"}}]}`))
	assert.Error(t, err, "duplicate canonical names must be rejected")

	_, err = catalog.Parse([]byte(`{"version":1,"services":[]}`))
	assert.Error(t, err)
}
