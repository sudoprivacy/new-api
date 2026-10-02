// sudoapi: Discover models, capabilities, and reference prices from trusted upstream catalogs.
package upstream_catalog

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const OptionKey = "UpstreamModelCatalog"

type Pricing struct {
	Currency                    string   `json:"currency"`
	Unit                        string   `json:"unit"`
	Input                       *float64 `json:"input"`
	Output                      *float64 `json:"output"`
	CacheRead                   *float64 `json:"cache_read"`
	CacheWrite5m                *float64 `json:"cache_write_5m"`
	CacheWrite1h                *float64 `json:"cache_write_1h"`
	LongContextThreshold        int      `json:"long_context_threshold,omitempty"`
	LongContextInputMultiplier  float64  `json:"long_context_input_multiplier,omitempty"`
	LongContextOutputMultiplier float64  `json:"long_context_output_multiplier,omitempty"`
}

type Entry struct {
	ChannelID            int      `json:"channel_id"`
	ContextWindow        int      `json:"context_window,omitempty"`
	MaxOutputTokens      int      `json:"max_output_tokens,omitempty"`
	VisionSupported      *bool    `json:"vision_supported,omitempty"`
	Pricing              *Pricing `json:"reference_pricing,omitempty"`
	ToolCallingSupported *bool    `json:"tool_calling_supported,omitempty"`
}

var catalog = struct {
	sync.RWMutex
	models map[string]Entry
}{models: map[string]Entry{}}

func Get(name string) (Entry, bool) {
	catalog.RLock()
	defer catalog.RUnlock()
	e, ok := catalog.models[name]
	return e, ok
}

func Decode(raw string) (map[string]Entry, error) {
	entries := map[string]Entry{}
	if err := common.UnmarshalJsonStr(raw, &entries); err != nil {
		return nil, err
	}
	for name, entry := range entries {
		if strings.TrimSpace(name) == "" || entry.ContextWindow < 0 || entry.MaxOutputTokens < 0 {
			return nil, fmt.Errorf("invalid upstream model metadata")
		}
		if entry.Pricing != nil {
			if _, err := entry.Pricing.Expression(); err != nil {
				return nil, fmt.Errorf("invalid upstream pricing for %s: %w", name, err)
			}
		}
	}
	return entries, nil
}

func Update(raw string) error {
	entries, err := Decode(raw)
	if err != nil {
		return err
	}
	catalog.Lock()
	defer catalog.Unlock()
	catalog.models = entries
	return nil
}

func BillingExpression(name string) (string, bool) {
	e, ok := Get(name)
	if !ok || e.Pricing == nil {
		return "", false
	}
	expr, err := e.Pricing.Expression()
	return expr, err == nil
}

// Expression uses explicit prices for both cache durations. No family defaults
// or fixed relationship between 5m and 1h prices is inferred.
func (p *Pricing) Expression() (string, error) {
	if p.Currency != "USD" || p.Unit != "per_million_tokens" {
		return "", fmt.Errorf("unsupported price unit")
	}
	for _, value := range []*float64{p.Input, p.Output, p.CacheRead, p.CacheWrite5m, p.CacheWrite1h} {
		if value == nil || *value < 0 || *value > 1e6 || math.IsNaN(*value) || math.IsInf(*value, 0) {
			return "", fmt.Errorf("missing or invalid token price")
		}
	}
	cost := func(in, out float64) string {
		return fmt.Sprintf("p * %.12g + c * %.12g + cr * %.12g + cc * %.12g + cc1h * %.12g", *p.Input*in, *p.Output*out, *p.CacheRead*in, *p.CacheWrite5m*in, *p.CacheWrite1h*in)
	}
	base := "tier(\"standard\", " + cost(1, 1) + ")"
	if p.LongContextThreshold == 0 {
		return base, nil
	}
	if p.LongContextThreshold < 0 || p.LongContextInputMultiplier < 1 || p.LongContextInputMultiplier > 100 || p.LongContextOutputMultiplier < 1 || p.LongContextOutputMultiplier > 100 || math.IsNaN(p.LongContextInputMultiplier) || math.IsNaN(p.LongContextOutputMultiplier) {
		return "", fmt.Errorf("invalid long-context pricing")
	}
	return fmt.Sprintf("len <= %d ? %s : tier(\"long_context\", %s)", p.LongContextThreshold, base, cost(p.LongContextInputMultiplier, p.LongContextOutputMultiplier)), nil
}
