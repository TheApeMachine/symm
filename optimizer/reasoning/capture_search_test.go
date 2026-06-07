package reasoning

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/optimizer/io"
	"github.com/theapemachine/symm/optimizer/replay"
)

func TestSearchCaptureRealizedBounded(t *testing.T) {
	capturePath := filepath.Join("..", "..", "runs", "capture.jsonl")

	if _, err := os.Stat(capturePath); err != nil {
		t.Skip("capture tape not present")
	}

	convey.Convey("Given 50k rows of live capture", t, func() {
		testconfig.Load(t)
		viper.Set("trading.replay.execution_stress_enabled", false)

		rows, _, err := io.LoadMeasurements(capturePath)

		convey.So(err, convey.ShouldBeNil)

		if len(rows) > 50_000 {
			rows = rows[:50_000]
		}

		costs := replay.DefaultReplayCosts()
		rules, _, rulesErr := broker.LoadInstrumentRulesFromKraken(t.Context())

		if rulesErr == nil {
			costs.InstrumentRules = rules
		}

		result, searchErr := Search(context.Background(), rows, costs, SearchConfig{
			BeamWidth: 8,
			MaxRounds: 6,
			Patience:  2,
		})

		convey.So(searchErr, convey.ShouldBeNil)

		convey.Convey("Best candidate P&L stays within plausible capital bounds", func() {
			t.Logf(
				"capture search: realized_eur=%.2f return=%.4f trades=%d evaluated=%d",
				result.Best.RealizedEUR,
				result.Best.Return,
				result.Best.Trades,
				result.Evaluated,
			)

			maxPlausible := costs.StartingCapital * 100

			if maxPlausible <= 0 {
				maxPlausible = replay.DefaultStartingCapital * 100
			}

			convey.So(result.Best.RealizedEUR, convey.ShouldBeLessThan, maxPlausible)
		})
	})
}
