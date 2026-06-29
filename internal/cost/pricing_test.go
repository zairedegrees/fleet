package cost

import "testing"

func TestCostUSDKnownModels(t *testing.T) {
	// 1M of each component on opus: 5 + 25 + 0.5 + 6.25 = 36.75
	u := Usage{In: 1_000_000, Out: 1_000_000, CacheRead: 1_000_000, CacheCreate: 1_000_000}
	usd, known := CostUSD(u, "claude-opus-4-8")
	if !known {
		t.Fatal("opus must be a known model")
	}
	if usd < 36.74 || usd > 36.76 {
		t.Errorf("opus 1M-each = $%.4f, want 36.75", usd)
	}
}

func TestCostUSDUnknownModelIsNeverFaked(t *testing.T) {
	if _, known := CostUSD(Usage{In: 100}, "gpt-4o"); known {
		t.Error("unknown model must return known=false, never a faked price")
	}
}

func TestCostUSDMatchesModelFamily(t *testing.T) {
	// haiku 1M input only = $1.00
	usd, known := CostUSD(Usage{In: 1_000_000}, "claude-haiku-4-5")
	if !known || usd < 0.99 || usd > 1.01 {
		t.Errorf("haiku 1M input = $%.4f known=%v, want 1.00 true", usd, known)
	}
}
