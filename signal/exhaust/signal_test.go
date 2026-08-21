package exhaust

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestExhaustNumber(t *testing.T) {
	Convey("Given trades on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendTrade(kraken.TradeData{
			Symbol: "AAA/USD", Side: "buy",
			Price: *decimal.NewFromInt64(100), Qty: 1,
			TradeID: 1, Timestamp: time.Unix(1_700_001_000, 0).UTC(),
		})

		Convey("It should emit an exhaustion z-score reading", func() {
			measurements := []*nmtypes.Measurement{}

			time.Sleep(50 * time.Millisecond)
			for measurement := range market.MarketMeasurements(
				market.MeasurementConsumers[types.MeasurementConsumerCategory],
			) {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, string(types.SourceExhaustion))
		})
	})
}
