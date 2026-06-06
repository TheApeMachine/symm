package reasoning

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func productionPlaybookPath(testingObject testing.TB) string {
	testingObject.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		testingObject.Fatal("runtime.Caller failed")
	}

	return filepath.Join(filepath.Dir(file), "..", "cfg", "perspectives.yaml")
}

func loadProductionPlaybook(testingObject *testing.T) []Thought {
	testingObject.Helper()

	raw, err := os.ReadFile(productionPlaybookPath(testingObject))
	if err != nil {
		testingObject.Fatalf("read production playbook: %v", err)
	}

	thoughts, err := ParseThoughts(raw)
	if err != nil {
		testingObject.Fatalf("parse production playbook: %v", err)
	}

	return thoughts
}

func collectActs(thoughts []Thought) []Act {
	var acts []Act

	var walk func([]Thought)
	walk = func(nodes []Thought) {
		for _, node := range nodes {
			if node.Do.Type != ActionNone {
				acts = append(acts, node.Do)
			}

			walk(node.Then)
		}
	}

	walk(thoughts)

	return acts
}

func hasEntryAction(acts []Act) bool {
	for _, act := range acts {
		if IsEntryAction(act.Type) {
			return true
		}
	}

	return false
}

func hasProtectiveAction(acts []Act) bool {
	for _, act := range acts {
		switch act.Type {
		case ActionSettlePosition, ActionStopLoss, ActionStopLossLimit,
			ActionTakeProfit, ActionTakeProfitLimit, ActionTrailingStop:
			return true
		default:
			continue
		}
	}

	return false
}

func signalSnapshot(category types.CategoryType, snr, last float64) types.Measurement {
	return types.Measurement{
		Category: category,
		SNR:      snr,
		Last:     last,
		Volume:   1_000_000,
	}
}

func flatPosition() PositionState {
	return PositionState{}
}

func heldPosition() PositionState {
	return PositionState{
		Holding:    true,
		EntryPrice: 100,
		Peak:       101,
		Last:       100.5,
	}
}

func productionContext(
	position PositionState, snapshots []types.Measurement,
) *WindowReason {
	return NewWindowReason(snapshots, types.RegimeTrending, position)
}

func TestProductionPlaybookContract(testingObject *testing.T) {
	Convey("Given the production playbook loaded from perspectives.yaml", testingObject, func() {
		playbook := loadProductionPlaybook(testingObject)
		acts := collectActs(playbook)

		Convey("No node opens a short position", func() {
			for _, act := range acts {
				So(IsShortAct(act), ShouldBeFalse)
			}
		})

		Convey("It should contain an entry and protective management", func() {
			So(hasEntryAction(acts), ShouldBeTrue)
			So(hasProtectiveAction(acts), ShouldBeTrue)
		})

		Convey("Extreme scarcity at threshold fires a market entry", func() {
			context := productionContext(
				flatPosition(),
				[]types.Measurement{
					signalSnapshot(types.CategoryExtremeScarcity, 1.0, 100),
				},
			)

			act, found := EvaluateStateful(playbook, context, NewReasonState())

			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionMarket)
			So(act.Fraction, ShouldEqual, 0.25)
		})

		Convey("Extreme scarcity below threshold does not enter", func() {
			context := productionContext(
				flatPosition(),
				[]types.Measurement{
					signalSnapshot(types.CategoryExtremeScarcity, 0.5, 100),
				},
			)

			_, found := EvaluateStateful(playbook, context, NewReasonState())

			So(found, ShouldBeFalse)
		})

		Convey("A held position receives the tuned trailing stop", func() {
			context := productionContext(heldPosition(), nil)

			act, found := EvaluateStateful(playbook, context, NewReasonState())

			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionTrailingStop)
			So(act.Offset, ShouldEqual, 0.01)
		})
	})
}
