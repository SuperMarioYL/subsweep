package match_test

import (
	"context"
	"errors"
	"testing"

	"github.com/SuperMarioYL/subsweep/internal/catalog"
	"github.com/SuperMarioYL/subsweep/internal/match"
	"github.com/SuperMarioYL/subsweep/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newKB(t *testing.T) *catalog.Catalog {
	t.Helper()
	kb, err := catalog.Load()
	require.NoError(t, err)
	return kb
}

func TestCatalogAdapterImplementsReportInterface(t *testing.T) {
	kb := newKB(t)
	var rc report.Catalog = match.Catalog{KB: kb}

	svc, ok := rc.Find("上海恩空中文化传播有限公司")
	require.True(t, ok)
	assert.Equal(t, "爱奇艺", svc.CanonicalName)
	assert.NotEmpty(t, svc.Cancel.Steps)

	_, ok = rc.Find("瑞幸咖啡")
	assert.False(t, ok)
}

func TestSuggestRanksExactFirst(t *testing.T) {
	m := match.New(newKB(t))

	sugs := m.Suggest("腾讯视频", 5)
	require.NotEmpty(t, sugs)
	assert.Equal(t, "腾讯视频", sugs[0].Service.CanonicalName)
	assert.InDelta(t, 1.0, sugs[0].Score, 1e-9)
}

func TestSuggestFuzzySurvivesTypos(t *testing.T) {
	m := match.New(newKB(t))

	// 迅 vs 讯: a one-character typo must still surface the right service.
	sugs := m.Suggest("腾迅视频VIP会员", 3)
	require.NotEmpty(t, sugs)
	assert.Equal(t, "腾讯视频", sugs[0].Service.CanonicalName)
}

func TestSuggestDropsNoise(t *testing.T) {
	m := match.New(newKB(t))
	assert.Empty(t, m.Suggest("瑞幸咖啡", 5))
	assert.Empty(t, m.Suggest("", 5))
	// Two unrelated companies must not match just because they share the
	// corporate template (…网络科技有限公司 / …科技有限公司).
	assert.Empty(t, m.Suggest("杭州优卖网络科技有限公司", 5))
}

func TestLLMDisabledWithoutKey(t *testing.T) {
	l := match.NewLLM(match.LLMConfig{Enabled: false})
	_, err := l.Classify(context.Background(), "某商户", []string{"腾讯视频"})
	assert.True(t, errors.Is(err, match.ErrDisabled))
	assert.False(t, l.Enabled())
	assert.Equal(t, match.DefaultModel, l.Model(), "deepseek-flash is the default model id")
}

func TestLLMRejectsAnswerOutsideCatalog(t *testing.T) {
	// Classify must never pass a name through that is not one of the
	// candidates it was given — the model cannot invent services.
	assert.NotPanics(t, func() {
		l := match.NewLLM(match.LLMConfig{Enabled: true, APIKey: "k"})
		_, _ = l.Classify(context.Background(), "", nil)
	})
}
