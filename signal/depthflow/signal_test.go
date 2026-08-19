package depthflow

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestDepthFlowNumber(t *testing.T) {
	Convey("Given trades on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")

		market.AppendTrade(kraken.TradeData{
			Symbol: "AAA/USD", Side: "buy",
			Price: *decimal.NewFromInt64(100), Qty: 1,
			TradeID: 1, Timestamp: time.Unix(1_700_001_000, 0).UTC(),
		})

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should emit a depth deviation reading", func() {
			measurements := []*nmtypes.Measurement{}

			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, string(types.SourceDepthFlow))
		})
	})
}
