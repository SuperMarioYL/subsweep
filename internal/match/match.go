// Package match resolves raw bill merchant names to canonical renewal
// services: dictionary hits against the embedded knowledge base, fuzzy
// candidates for near-miss names, and — off by default — an optional LLM
// assist that classifies merchants no local rule can match.
package match

import (
	"sort"
	"strings"

	"github.com/SuperMarioYL/subsweep/internal/catalog"
	"github.com/SuperMarioYL/subsweep/internal/report"
)

// minSuggestScore is the fuzzy-similarity floor below which a candidate is
// considered noise rather than a suggestion.
const minSuggestScore = 0.6

// Catalog adapts the embedded knowledge base to the narrow report.Catalog
// interface.
type Catalog struct {
	KB *catalog.Catalog
}

// Find resolves a raw merchant name to the report-facing service projection.
func (c Catalog) Find(merchant string) (report.Service, bool) {
	if c.KB == nil {
		return report.Service{}, false
	}
	svc, ok := c.KB.Find(merchant)
	if !ok {
		return report.Service{}, false
	}
	return toReportService(svc), true
}

func toReportService(svc catalog.Service) report.Service {
	return report.Service{
		CanonicalName: svc.CanonicalName,
		Cancel: report.CancelPath{
			App:   svc.Cancel.App,
			Steps: svc.Cancel.Steps,
			URL:   svc.Cancel.URL,
		},
	}
}

// Candidate is one ranked suggestion for a raw merchant name.
type Candidate struct {
	Service catalog.Service
	Score   float64
}

// Matcher ranks knowledge-base services against raw merchant names.
type Matcher struct {
	kb *catalog.Catalog
}

// New builds a Matcher over the given knowledge base.
func New(kb *catalog.Catalog) *Matcher { return &Matcher{kb: kb} }

// Suggest returns up to limit candidate services for a raw merchant name,
// best first: exact canonical/alias hits score 1.0, keyword containment and
// fuzzy similarity land in between, and anything below the noise floor is
// dropped.
func (m *Matcher) Suggest(merchant string, limit int) []Candidate {
	if m == nil || m.kb == nil || limit <= 0 {
		return nil
	}
	target := catalog.Normalize(merchant)
	if target == "" {
		return nil
	}

	var out []Candidate
	for _, svc := range m.kb.Services() {
		score := 0.0
		if catalog.Normalize(svc.CanonicalName) == target {
			score = 1.0
		} else if s := scoreFor(target, svc.CanonicalName); s > score {
			score = s
		}
		for _, alias := range svc.Aliases {
			if s := scoreFor(target, alias); s > score {
				score = s
			}
		}
		for _, kw := range svc.Keywords {
			if n := catalog.Normalize(kw); len([]rune(n)) >= 2 && strings.Contains(target, n) {
				score = max(score, 0.85)
			}
		}
		if score < minSuggestScore {
			continue
		}
		out = append(out, Candidate{Service: svc, Score: score})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Service.CanonicalName < out[j].Service.CanonicalName
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// scoreFor scores one name against the normalized target: an exact alias
// match is near-certain, otherwise fuzzy similarity applies.
func scoreFor(target, name string) float64 {
	n := catalog.Normalize(name)
	if n == "" {
		return 0
	}
	if n == target {
		return 0.98
	}
	if strings.Contains(target, n) || strings.Contains(n, target) {
		return 0.75
	}
	return similarity(stripBoilerplate(target), stripBoilerplate(n))
}

// companySuffixes carry no identity signal — comparing without them keeps
// two unrelated companies that merely share the corporate template
// (…网络科技有限公司 / …科技有限公司) from scoring as near-matches.
var companySuffixes = []string{"股份有限公司", "有限责任公司", "有限公司"}

func stripBoilerplate(s string) string {
	for _, suffix := range companySuffixes {
		s = strings.ReplaceAll(s, suffix, "")
	}
	return s
}

// similarity maps edit distance onto 0..1 over the longer string.
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	longer := max(len(ra), len(rb))
	return 1 - float64(editDistance(ra, rb))/float64(longer)
}

func editDistance(a, b []rune) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(min(curr[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
