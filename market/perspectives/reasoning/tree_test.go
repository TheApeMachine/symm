package reasoning

import (
	"testing"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

type fakeReasonContext struct {
	regime  types.Regime
	holding bool
	signals map[types.CategoryType]float64
}

func (ctx fakeReasonContext) Regime() types.Regime { return ctx.regime }

func (ctx fakeReasonContext) PositionSide() trading.Side { return "" }

func (ctx fakeReasonContext) Lifecycle(state types.ObservationType) bool {
	switch state {
	case types.ObservationHolding:
		return ctx.holding
	case types.ObservationNotHolding:
		return !ctx.holding
	default:
		return false
	}
}

func (ctx fakeReasonContext) Signal(category types.CategoryType, _ UnitType, _ int) (float64, bool) {
	value, ok := ctx.signals[category]

	return value, ok
}

func (ctx fakeReasonContext) Scalar(_ Subject, _ UnitType, _ int) (float64, bool) {
	return 0, false
}

func sampleThoughts() []Thought {
	return []Thought{
		{
			When: Predicate{Subject: SubjectRegime, Op: ComparisonEquals, Regime: types.RegimeBullish},
			Then: []Thought{
				{
					When: Predicate{
						Subject:  SubjectSignal,
						Category: "endogenous_alpha",
						Unit:     UnitSNR,
						Op:       ComparisonAtLeast,
						Value:    1.0,
					},
					Do: Act{Type: ActionLimit},
				},
			},
		},
		{
			When: Predicate{Subject: SubjectRegime, Op: ComparisonEquals, Regime: types.RegimeChoppy},
		},
	}
}

func TestBuildTreeKeysAndLabels(t *testing.T) {
	tree := BuildTree(sampleThoughts())

	if len(tree) != 3 {
		t.Fatalf("tree has %d nodes, want 3: %+v", len(tree), tree)
	}

	byKey := map[string]TreeNode{}

	for _, node := range tree {
		byKey[node.Key] = node
	}

	if node, ok := byKey["0"]; !ok || node.Depth != 0 || node.Parent != "" {
		t.Errorf("node 0 wrong: %+v", node)
	}

	if node, ok := byKey["0.0"]; !ok || node.Depth != 1 || node.Parent != "0" || node.Action != "limit" {
		t.Errorf("node 0.0 wrong: %+v", node)
	}
}

func TestDescribePredicate(t *testing.T) {
	cases := map[string]Predicate{
		"regime = bullish": {Subject: SubjectRegime, Op: ComparisonEquals, Regime: types.RegimeBullish},
		"signal endogenous_alpha.snr ≥ 1": {
			Subject: SubjectSignal, Category: "endogenous_alpha", Unit: UnitSNR,
			Op: ComparisonAtLeast, Value: 1.0,
		},
	}

	for want, predicate := range cases {
		if got := DescribePredicate(predicate); got != want {
			t.Errorf("DescribePredicate = %q, want %q", got, want)
		}
	}
}

func TestEvaluateStatefulTracedRecordsOutcomes(t *testing.T) {
	thoughts := sampleThoughts()

	ctx := fakeReasonContext{
		regime:  types.RegimeBullish,
		holding: false,
		signals: map[types.CategoryType]float64{"endogenous_alpha": 2.0},
	}

	trace := &ReasonTrace{}
	act, found := EvaluateStatefulTraced(thoughts, ctx, NewReasonState(), trace)

	if !found || act.Type != ActionLimit {
		t.Fatalf("expected ActionLimit, got found=%v act=%v", found, act.Type)
	}

	// The untraced evaluator must agree exactly.
	plainAct, plainFound := EvaluateStateful(thoughts, ctx, NewReasonState())

	if plainFound != found || plainAct.Type != act.Type {
		t.Errorf("traced and untraced disagree: %v/%v vs %v/%v", found, act.Type, plainFound, plainAct.Type)
	}

	byKey := map[string]NodeTrace{}

	for _, node := range trace.Nodes {
		byKey[node.Key] = node
	}

	if node := byKey["0"]; !node.Reachable || !node.Fires {
		t.Errorf("node 0 should be reachable and fire: %+v", node)
	}

	if node := byKey["0.0"]; !node.Reachable || !node.Fires || !node.Fired {
		t.Errorf("node 0.0 should fire and be the chosen action: %+v", node)
	}

	if node := byKey["1"]; !node.Reachable || node.Fires {
		t.Errorf("node 1 (choppy) should be reachable but not fire: %+v", node)
	}
}
