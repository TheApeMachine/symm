package trader

import (
	"math"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

/*
fieldAndCausal seeds a symbol with an organized field and a given causal uplift
so it is a viable entry whose score scales with the uplift.
*/
func fieldAndCausal(symbol string, uplift float64) []*datura.Artifact {
	return []*datura.Artifact{
		manifoldMeasurement(symbol, 0.8, 0.7, 0.1, 0.05),
		causalMeasurement(symbol, uplift),
	}
}

func TestAllocationCapsNormalSlots(t *testing.T) {
	viper.Reset()
	viper.Set("trading.slots.normal", 2)
	t.Cleanup(viper.Reset)

	decider := NewDecider()

	// Five viable, ordinary entries (all equal scores → no standout, so the
	// reserved slots stay empty); only two normal slots exist and none are held,
	// so exactly two of the five should be admitted.
	measurements := make([]*datura.Artifact, 0)
	actions := make([]*datura.Artifact, 0)

	for _, symbol := range []string{"A/USD", "B/USD", "C/USD", "D/USD", "E/USD"} {
		measurements = append(measurements, fieldAndCausal(symbol, 1.0)...)
		actions = append(actions, candidate(symbol, logic.SideBuy, logic.ActionMarket, 0.6))
	}

	chosen, _ := decider.choose(measurements, actions, nil)

	if len(chosen) != 2 {
		t.Fatalf("expected 2 admitted (normal slots), got %d", len(chosen))
	}
}

func TestAllocationReservedSlotOnlyForElite(t *testing.T) {
	viper.Reset()
	viper.Set("trading.slots.normal", 1)
	viper.Set("trading.slots.reserved", 1)
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(viper.Reset)

	decider := NewDecider()

	// The single normal slot is already occupied by an open position, so every
	// fresh candidate is overflow. Three ordinary entries plus one explosive
	// outlier (huge uplift) compete for the one reserved slot — it must admit
	// ONLY the outlier; the ordinary overflow stays out, even though the normal
	// slots are full (the sudden-pump case the reserved slots exist for).
	balances := &logic.Balances{
		Asset: []logic.BalanceAsset{{Asset: "OPEN", Balance: 1.0}},
	}

	measurements := make([]*datura.Artifact, 0)
	actions := make([]*datura.Artifact, 0)

	for _, symbol := range []string{"A/USD", "B/USD", "C/USD"} {
		measurements = append(measurements, fieldAndCausal(symbol, 1.0)...)
		actions = append(actions, candidate(symbol, logic.SideBuy, logic.ActionMarket, 0.6))
	}

	measurements = append(measurements, fieldAndCausal("PUMP/USD", 50.0)...)
	actions = append(actions, candidate("PUMP/USD", logic.SideBuy, logic.ActionMarket, 0.9))

	chosen, _ := decider.choose(measurements, actions, balances)

	symbols := make([]string, 0, len(chosen))

	for _, action := range chosen {
		symbol, _ := action.Scope()
		symbols = append(symbols, symbol)
	}

	if len(chosen) != 1 {
		t.Fatalf("expected only the elite outlier in the reserved slot, got %d: %v", len(chosen), symbols)
	}

	if !containsSymbol(symbols, "PUMP/USD") {
		t.Fatalf("explosive outlier did not claim the reserved slot: %v", symbols)
	}
}

func TestAllocationHonorsMaxConcurrentPositions(t *testing.T) {
	viper.Reset()
	viper.Set("trading.max_concurrent_positions", 4)
	viper.Set("trading.slots.normal", 10)
	viper.Set("trading.slots.reserved", 0)
	t.Cleanup(viper.Reset)

	alloc := newAllocation()
	entries := make([]rankedEntry, 0, 6)

	for index, score := range []float64{6, 5, 4, 3, 2, 1} {
		action := datura.Acquire("story", datura.APPJSON).
			WithRole("buy").
			WithScope(string(rune('A'+index)) + "/USD")
		entries = append(entries, rankedEntry{
			action:     action,
			score:      score,
			confidence: 1,
		})
	}

	admitted := alloc.admit(entries, nil)

	if len(admitted) != 4 {
		t.Fatalf("expected max_concurrent_positions=4 to cap admissions, got %d", len(admitted))
	}
}

func TestAllocationHonorsExplicitZeroReservedSlots(t *testing.T) {
	viper.Reset()
	viper.Set("trading.max_concurrent_positions", 2)
	viper.Set("trading.slots.normal", 1)
	viper.Set("trading.slots.reserved", 0)
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(viper.Reset)

	alloc := newAllocation()
	action := datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope("PUMP/USD")
	entries := []rankedEntry{{
		action:     action,
		score:      100,
		confidence: 1,
	}}
	balances := &logic.Balances{
		Asset: []logic.BalanceAsset{{Asset: "OPEN", Balance: 1.0}},
	}

	admitted := alloc.admit(entries, balances)

	if len(admitted) != 0 {
		t.Fatalf("expected explicit reserved=0 to reject overflow entry, got %d", len(admitted))
	}
}

func TestAllocationRiskSizesByConfidence(t *testing.T) {
	viper.Reset()
	viper.Set("trading.slots.normal", 4)
	viper.Set("trading.sizing.base_fraction", 0.10)
	t.Cleanup(viper.Reset)

	decider := NewDecider()

	measurements := fieldAndCausal("HALF/USD", 1.0)
	actions := []*datura.Artifact{
		candidate("HALF/USD", logic.SideBuy, logic.ActionMarket, 0.5),
	}

	chosen, _ := decider.choose(measurements, actions, nil)

	if len(chosen) != 1 {
		t.Fatalf("expected 1 admitted, got %d", len(chosen))
	}

	// base 0.10 scaled by 0.5 confidence = 0.05 risk fraction on the action.
	fraction := datura.Peek[float64](chosen[0], "fraction")
	offset := datura.Peek[float64](chosen[0], "offset")

	if math.Abs(fraction-0.05) > 1e-9 {
		t.Fatalf("expected risk fraction 0.05, got %v", fraction)
	}

	if math.Abs(offset-0.05) > 1e-9 {
		t.Fatalf("expected stop offset 0.05, got %v", offset)
	}
}

func TestAllocationPreservesExplicitStopOffset(t *testing.T) {
	viper.Reset()
	viper.Set("trading.slots.normal", 4)
	viper.Set("trading.sizing.base_fraction", 0.10)
	t.Cleanup(viper.Reset)

	decider := NewDecider()

	measurements := fieldAndCausal("WIDE/USD", 1.0)
	action := candidate("WIDE/USD", logic.SideBuy, logic.ActionMarket, 0.5)
	action.Merge("offset", 0.20)

	chosen, _ := decider.choose(measurements, []*datura.Artifact{action}, nil)

	if len(chosen) != 1 {
		t.Fatalf("expected 1 admitted, got %d", len(chosen))
	}

	offset := datura.Peek[float64](chosen[0], "offset")

	if math.Abs(offset-0.20) > 1e-9 {
		t.Fatalf("expected explicit stop offset 0.20, got %v", offset)
	}
}

func TestDeciderGatesNonFiniteScore(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	decider := NewDecider()

	// A NaN-inducing field (zero coherence and guidance give edge 0 → gated by
	// edge anyway) is not enough; force the field edge positive but feed an
	// infinite uplift so the score is +Inf, which must still be rejected.
	measurements := []*datura.Artifact{
		manifoldMeasurement("INF/USD", 0.8, 0.7, 0.1, 0.05),
		causalMeasurement("INF/USD", math.Inf(1)),
	}

	actions := []*datura.Artifact{
		candidate("INF/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	chosen, _ := decider.choose(measurements, actions, nil)

	if len(chosen) != 0 {
		t.Fatalf("non-finite score was not gated: %d admitted", len(chosen))
	}
}
