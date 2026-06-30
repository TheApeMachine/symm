package optimizer

import (
	"strings"
	"testing"
)

func TestReadJSONLParsesDecisionFrames(t *testing.T) {
	input := strings.NewReader(`{"decisions":[{"symbol":"BTC/USD","type":"limit","source":"fluid","category":"laminar","confidence":0.7,"edge":0.03,"hurdle":0.004,"fill_probability":0.8},{"symbol":"ETH/USD","type":"market","source":"toxicity","category":"vacuum","reward":"-0.01","friction":"0.004"}]}`)

	samples, err := ReadJSONL(input)
	if err != nil {
		t.Fatalf("ReadJSONL returned error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(samples))
	}
	if samples[0].Symbol != "BTC/USD" || samples[0].Source != "fluid" {
		t.Fatalf("first sample did not preserve decision fields: %#v", samples[0])
	}
	if samples[1].Reward != -0.01 || samples[1].Friction != 0.004 {
		t.Fatalf("second sample did not parse numeric strings: %#v", samples[1])
	}
}

func TestOptimizeUsesMCTSWithFrictionAndFill(t *testing.T) {
	yes := true
	samples := []Sample{
		{Type: "limit", Source: "fluid", Category: "laminar", Confidence: 0.8, Edge: 0.06, Friction: 0.005, FillProbability: 0.9},
		{Type: "limit", Source: "fluid", Category: "laminar", Confidence: 0.7, Edge: 0.05, Friction: 0.005, FillProbability: 0.8},
		{Type: "market", Source: "pumpdump", Category: "ignition", Confidence: 0.9, Edge: 0.08, Friction: 0.05, Filled: &yes},
		{Type: "market", Source: "pumpdump", Category: "ignition", Confidence: 0.8, Edge: 0.07, Friction: 0.05, Filled: &yes},
	}

	report, err := Optimize(samples, Options{
		Iterations:      64,
		HoldoutFraction: 0,
		Exploration:     1,
		CausalAlpha:     1,
	})
	if err != nil {
		t.Fatalf("Optimize returned error: %v", err)
	}
	if report.Best.Source != "fluid" || report.Best.Category != "laminar" {
		t.Fatalf("best = %#v, want fluid/laminar because net fill-adjusted reward beats high-friction market entry", report.Best)
	}
	if report.UsableSamples != len(samples) {
		t.Fatalf("usable samples = %d, want %d", report.UsableSamples, len(samples))
	}
}

func TestOptimizeRejectsImplicitLimitFill(t *testing.T) {
	_, err := Optimize([]Sample{
		{Type: "limit", Source: "fluid", Category: "laminar", Edge: 0.03},
	}, Options{})
	if err == nil {
		t.Fatal("Optimize succeeded without explicit limit fill data")
	}
}

func TestOptimizeAcceptsBackendPricedExpectedEdge(t *testing.T) {
	report, err := Optimize([]Sample{
		{Type: "limit", Source: "fluid", Category: "laminar", Edge: 0.03, Friction: 0.004, EconomicPriced: true},
	}, Options{Iterations: 1, HoldoutFraction: 0})
	if err != nil {
		t.Fatalf("Optimize returned error for backend-priced edge: %v", err)
	}
	if report.Best.TrainReward != 0.026 {
		t.Fatalf("train reward = %v, want 0.026", report.Best.TrainReward)
	}
}

func TestRewriteTreeYAMLReordersMatchingEntryBranches(t *testing.T) {
	input := []byte(`
branches:
  - condition_group:
      conditions:
        - type: is_true
          left:
            type: holding
            holding:
              held: true
    action:
      type: settle_position
      side: sell
  - condition_group:
      conditions:
        - type: is_true
          left:
            source: pumpdump
            type: category
            category:
              type: vertical_ignition
    action:
      type: market
      side: buy
  - condition_group:
      conditions:
        - type: is_true
          left:
            source: fluid
            type: category
            category:
              type: laminar
    action:
      type: limit
      side: buy
`)

	out, rewrite, err := RewriteTreeYAML(input, Report{
		Recommendations: []Recommendation{
			{Type: "limit", Source: "fluid", Category: "laminar"},
			{Type: "market", Source: "pumpdump", Category: "vertical_ignition"},
		},
	})
	if err != nil {
		t.Fatalf("RewriteTreeYAML returned error: %v", err)
	}
	if rewrite.BranchesMatched != 2 {
		t.Fatalf("matched branches = %d, want 2", rewrite.BranchesMatched)
	}

	text := string(out)
	exitIndex := strings.Index(text, "settle_position")
	fluidIndex := strings.Index(text, "laminar")
	pumpIndex := strings.Index(text, "vertical_ignition")
	if exitIndex < 0 || fluidIndex < 0 || pumpIndex < 0 {
		t.Fatalf("rewritten tree missing expected branches:\n%s", text)
	}
	if !(exitIndex < fluidIndex && fluidIndex < pumpIndex) {
		t.Fatalf("branch order wrong; want exit before optimized fluid before pumpdump:\n%s", text)
	}
}
