package replay

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal/testconfig"
	preasoning "github.com/theapemachine/symm/market/perspectives/reasoning"
	ptypes "github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/optimizer/io"
)

func TestReplayCaptureRealizedSanity(t *testing.T) {
	capturePath := filepath.Join("..", "..", "runs", "capture.jsonl")

	if _, err := os.Stat(capturePath); err != nil {
		t.Skip("capture tape not present")
	}

	convey.Convey("Given the first 50k rows of the live capture", t, func() {
		testconfig.Load(t)
		viper.Set("trading.replay.execution_stress_enabled", false)

		rows, _, err := io.LoadMeasurements(capturePath)

		convey.So(err, convey.ShouldBeNil)

		if len(rows) > 50_000 {
			rows = rows[:50_000]
		}

		costs := DefaultReplayCosts()
		rules, _, rulesErr := broker.LoadInstrumentRulesFromKraken(t.Context())

		if rulesErr == nil {
			costs.InstrumentRules = rules
		}

		tape, compileErr := PrecompileTapeWorkers(rows, 4)

		convey.So(compileErr, convey.ShouldBeNil)

		forest := []preasoning.Thought{{
			When: preasoning.Predicate{
				All: []preasoning.Predicate{{
					Subject:  preasoning.SubjectSignal,
					Category: ptypes.CategoryVerticalIgnition,
					Unit:     preasoning.UnitSNR,
					Op:       preasoning.ComparisonAtLeast,
					Value:    1,
				}},
			},
			Do: preasoning.Act{Type: preasoning.ActionMarket},
		}}

		result := NewThoughtSimulation(context.Background(), forest, tape, costs).Result()

		convey.Convey("Realized P&L should stay within deployable capital bounds", func() {
			t.Logf(
				"capture sanity: realized_eur=%.2f return=%.4f trades=%d starting=%.0f",
				result.RealizedEUR,
				result.Score,
				result.ClosedTrades,
				result.StartingCapital,
			)

			maxPlausible := result.StartingCapital * 100

			convey.So(result.RealizedEUR, convey.ShouldBeLessThan, maxPlausible)
		})
	})
}
