// Package catalog embeds the curated renewal-service knowledge base: the
// canonical name, billing aliases, distinctive keywords and the
// human-verified official cancel path for each high-frequency Chinese
// subscription service. Cancel paths come only from this hand-maintained
// file — never from an LLM.
package catalog

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"
)

//go:embed data/services.json
var dataFS embed.FS

// CancelPath is the official way to turn off one service's auto-renewal.
type CancelPath struct {
	App   string   `json:"app"`
	Steps []string `json:"steps"`
	URL   string   `json:"url"`
}

// Service is one knowledge-base entry.
type Service struct {
	CanonicalName string     `json:"canonical_name"`
	Aliases       []string   `json:"aliases,omitempty"`
	Keywords      []string   `json:"keywords,omitempty"`
	Cancel        CancelPath `json:"cancel"`
}

// fileFormat is the on-disk shape of data/services.json.
type fileFormat struct {
	Version  int       `json:"version"`
	Services []Service `json:"services"`
}

// Catalog is the loaded, searchable knowledge base. It is safe for
// concurrent read access after Load returns.
type Catalog struct {
	services []Service
	index    map[string]int // normalized canonical name or alias -> service index
}

// Load reads the knowledge base embedded in the binary.
func Load() (*Catalog, error) {
	raw, err := dataFS.ReadFile("data/services.json")
	if err != nil {
		return nil, fmt.Errorf("catalog: 读取内置知识库失败: %w", err)
	}
	return Parse(raw)
}

// Parse builds a Catalog from knowledge-base JSON. It rejects duplicate
// canonical names and aliases so lookups stay unambiguous.
func Parse(raw []byte) (*Catalog, error) {
	var file fileFormat
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("catalog: services.json 格式错误: %w", err)
	}
	if len(file.Services) == 0 {
		return nil, errors.New("catalog: services.json 未包含任何服务")
	}

	c := &Catalog{index: make(map[string]int, len(file.Services))}
	for i, svc := range file.Services {
		if strings.TrimSpace(svc.CanonicalName) == "" {
			return nil, fmt.Errorf("catalog: 第 %d 项缺少 canonical_name", i+1)
		}
		if strings.TrimSpace(svc.Cancel.App) == "" {
			return nil, fmt.Errorf("catalog: 服务 %q 缺少取消入口 app", svc.CanonicalName)
		}
		if _, dup := c.index[Normalize(svc.CanonicalName)]; dup {
			return nil, fmt.Errorf("catalog: 服务名 %q 重复", svc.CanonicalName)
		}
		for _, alias := range svc.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			if _, dup := c.index[Normalize(alias)]; dup {
				return nil, fmt.Errorf("catalog: 别名 %q 重复", alias)
			}
			c.index[Normalize(alias)] = i
		}
		c.index[Normalize(svc.CanonicalName)] = i
		c.services = append(c.services, svc)
	}
	return c, nil
}

// defaultCatalog backs the parser's known-merchant heuristic; the embedded
// file ships inside the binary, so a load error can only be a build bug.
var defaultCatalog = sync.OnceValue(func() *Catalog {
	c, err := Load()
	if err != nil {
		return &Catalog{index: map[string]int{}}
	}
	return c
})

// Default returns the embedded knowledge base, shared package-wide.
func Default() *Catalog { return defaultCatalog() }

// Find resolves a raw bill merchant name to a known renewal service: an
// exact canonical/alias match first, then the most distinctive keyword
// contained in the name. The second result is false when nothing matches.
func (c *Catalog) Find(merchant string) (Service, bool) {
	if c == nil || strings.TrimSpace(merchant) == "" {
		return Service{}, false
	}
	norm := Normalize(merchant)
	if norm == "" {
		return Service{}, false
	}
	if i, ok := c.index[norm]; ok {
		return c.services[i], true
	}
	best, bestLen := -1, 0
	for i, svc := range c.services {
		for _, kw := range svc.Keywords {
			n := Normalize(kw)
			if len([]rune(n)) < 2 {
				continue // too generic to be distinctive
			}
			if strings.Contains(norm, n) && len(n) > bestLen {
				best, bestLen = i, len(n)
			}
		}
	}
	if best >= 0 {
		return c.services[best], true
	}
	return Service{}, false
}

// Services returns every entry in file order.
func (c *Catalog) Services() []Service {
	if c == nil {
		return nil
	}
	out := make([]Service, len(c.services))
	copy(out, c.services)
	return out
}

// Size is the number of services in the knowledge base.
func (c *Catalog) Size() int {
	if c == nil {
		return 0
	}
	return len(c.services)
}

// Normalize reduces a merchant name or alias to the lookup key form:
// lowercase, with every character that is not an ASCII letter/digit or a
// CJK ideograph removed. "Apple Distribute." and "apple distribute" thus
// share one key.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', unicode.Is(unicode.Han, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}
