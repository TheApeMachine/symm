package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
TestStrategyDecision verifies the compact per-symbol strategy audit row retains
the friction-aware evidence needed to explain one planner outcome.
*/
func TestStrategyDecision(t *testing.T) {
	Convey("Given a strategy decision audit row", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		recorder, err := NewRecorder(path)
		So(err, ShouldBeNil)

		decision := types.Decision{
			Action:            types.ActionEnter,
			Symbol:            "BTC/USD",
			Utility:           0.12,
			ExpectedReturn:    decimal.NewFromFloat64(0.3),
			ExpectedFees:      decimal.NewFromFloat64(0.05),
			ExpectedSpread:    decimal.NewFromFloat64(0.04),
			ExpectedImpact:    decimal.NewFromFloat64(0.03),
			AdverseSelection:  0.02,
			Confidence:        0.7,
			Uncertainty:       0.01,
			ProposedNotional:  decimal.NewFromInt64(250),
			AvailableCapital:  decimal.NewFromInt64(1000),
			OpenPositions:     3,
			SlotCapacity:      4,
			ForecastSource:    "resonance+causal",
			ForecastEpoch:     42,
			AllocationClass:   "normal",
			Alternatives:      map[string]float64{"enter": 0.12, "nothing": 0},
			Cause:             "entry",
			Reason:            "executable utility exceeds doing nothing",
			OpportunityMargin: 0.05,
			CognitiveLead:     0.2,
			BasinConfidence:   0.3,
		}

		Convey("It should write one symbol-scoped evidence row", func() {
			So(StrategyDecision(recorder, 17, types.LifecycleManaging, decision), ShouldBeNil)
			So(recorder.Close(), ShouldBeNil)

			file, openErr := os.Open(path)
			So(openErr, ShouldBeNil)
			defer file.Close()

			scanner := bufio.NewScanner(file)
			So(scanner.Scan(), ShouldBeTrue)

			var decoded map[string]any
			So(json.Unmarshal(scanner.Bytes(), &decoded), ShouldBeNil)
			So(decoded["type"], ShouldEqual, "strategy_decision")

			value := decoded["value"].(map[string]any)
			So(value["tick"], ShouldEqual, float64(17))
			So(value["symbol"], ShouldEqual, "BTC/USD")
			So(value["lifecycle"], ShouldEqual, types.LifecycleManaging)
			So(value["action"], ShouldEqual, string(types.ActionEnter))
			So(value["expected_return"], ShouldEqual, "0.3")
			So(value["executable_return"], ShouldEqual, "0.16")
			So(value["available_capital"], ShouldEqual, "1000.000000000000")
		})
	})
}
