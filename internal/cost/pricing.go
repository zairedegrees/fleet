// Package cost computes measured token spend for fleet agents from their Claude
// Code transcripts. The pricing table is the single source of truth for the
// dollar weighting; an unpriced model yields known=false, never a faked $0.
package cost

import "strings"

// Usage is per-model accumulated token counts from one transcript.
type Usage struct {
	In          int64 // message.usage.input_tokens (uncached input)
	Out         int64 // output_tokens
	CacheRead   int64 // cache_read_input_tokens
	CacheCreate int64 // cache_creation_input_tokens
}

// Rates are USD per 1M tokens for one model family.
type Rates struct {
	Input, Output, CacheRead, CacheWrite float64
}

// lookupRates matches the transcript model string to a built-in rate set.
// Cache read = 0.10x input, cache write (5m TTL) = 1.25x input — prompt-caching
// economics, matching tools/token-benchmark/bench.py. Source rates: claude-api
// skill model table (cached 2026-06-04).
func lookupRates(model string) (Rates, bool) {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"): // 4.8 / 4.7 / 4.6 all $5 / $25
		return Rates{Input: 5, Output: 25, CacheRead: 0.5, CacheWrite: 6.25}, true
	case strings.Contains(m, "sonnet"):
		return Rates{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75}, true
	case strings.Contains(m, "haiku"):
		return Rates{Input: 1, Output: 5, CacheRead: 0.1, CacheWrite: 1.25}, true
	case strings.Contains(m, "fable"), strings.Contains(m, "mythos"):
		return Rates{Input: 10, Output: 50, CacheRead: 1, CacheWrite: 12.5}, true
	}
	return Rates{}, false
}

// CostUSD returns the cost-weighted dollar amount for u under model's rates.
// known is false when model has no entry in the table (caller renders "?").
func CostUSD(u Usage, model string) (float64, bool) {
	r, ok := lookupRates(model)
	if !ok {
		return 0, false
	}
	perM := func(tokens int64, rate float64) float64 {
		return float64(tokens) / 1_000_000 * rate
	}
	return perM(u.In, r.Input) + perM(u.Out, r.Output) +
		perM(u.CacheRead, r.CacheRead) + perM(u.CacheCreate, r.CacheWrite), true
}
