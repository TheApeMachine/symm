package toxicity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestToxicityNumber(t *testing.T) {
	Convey("Given level3 order frames on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		base := time.Unix(1_700_004_000, 0).UTC()

		market.AppendLevel3(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: base,
		})

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should emit an honesty reading per frame", func() {
			measurements := []*nmtypes.Measurement{}

			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, string(types.SourceToxicity))
		})
	})
}
