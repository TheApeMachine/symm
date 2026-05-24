package market

import "testing"

func TestDepthQuoteFillPartial(t *testing.T) {
	levels := []BookLevel{
		{Price: 100, Volume: 0.5},
	}

	result := DepthQuoteFill(levels, 100)

	if result.Complete {
		t.Fatal("expected partial fill")
	}

	if result.QuoteProceeds != 50 {
		t.Fatalf("expected proceeds 50, got %v", result.QuoteProceeds)
	}

	if result.BaseQty != 0.5 {
		t.Fatalf("expected base 0.5, got %v", result.BaseQty)
	}
}

func TestDepthBaseFillComplete(t *testing.T) {
	levels := []BookLevel{
		{Price: 100, Volume: 0.25},
		{Price: 99, Volume: 0.25},
	}

	result := DepthBaseFill(levels, 0.5)

	if !result.Complete {
		t.Fatal("expected complete base fill")
	}

	if result.BaseQty != 0.5 {
		t.Fatalf("expected base 0.5, got %v", result.BaseQty)
	}
}
