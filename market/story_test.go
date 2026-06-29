package market

import (
	"sync"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func TestStoryUpdateScopesActionArtifacts(t *testing.T) {
	story := &Story{
		symbols: &sync.Map{},
		tree: &logic.Tree{
			Branches: []*logic.Branch{{
				ConditionGroup: &logic.ConditionGroup{
					Boolean: logic.BooleanTypeAnd,
					Conditions: []logic.Condition{{
						Type: logic.ConditionIsTrue,
						Left: logic.ConditionOperand{
							Type:     logic.SubjectCategory,
							Source:   logic.SourcePumpDump,
							Category: logic.NewCategory(logic.CategoryVerticalIgnition),
						},
					}},
				},
				Action: &logic.Action{
					Type: logic.ActionMarket,
					Side: logic.SideBuy,
				},
			}},
		},
	}
	measurement := datura.Acquire("measurement", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope("BTC/USD")
	_ = measurement.SetOrigin(string(logic.SourcePumpDump))
	measurement.MergeOutput("value", float64(logic.CategoryIndex(logic.CategoryVerticalIgnition)))
	measurement.MergeOutput("confidence", 0.8)

	story.Update([]*datura.Artifact{measurement})
	actions := story.Actions(nil)

	if len(actions) != 1 {
		t.Fatalf("expected one story action, got %d", len(actions))
	}

	symbol, _ := actions[0].Scope()
	side, _ := actions[0].Role()

	if symbol != "BTC/USD" || side != "buy" {
		t.Fatalf("story action missing symbol or side: %s/%s", symbol, side)
	}

	if payloadSymbol := datura.Peek[string](actions[0], "symbol"); payloadSymbol != "BTC/USD" {
		t.Fatalf("story action payload symbol = %q, want BTC/USD", payloadSymbol)
	}

	if !datura.Peek[bool](measurement, "journey", "story", "evaluated") {
		t.Fatal("measurement was not stamped as story-evaluated")
	}

	if candidates := datura.Peek[float64](measurement, "journey", "story", "candidates"); candidates != 1 {
		t.Fatalf("measurement story candidates = %v, want 1", candidates)
	}

	if status := datura.Peek[string](actions[0], "journey", "story", "status"); status != "candidate" {
		t.Fatalf("action journey story status = %q, want candidate", status)
	}
}
