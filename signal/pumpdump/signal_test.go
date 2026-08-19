package pumpdump

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

func TestPumpDumpNumber(t *testing.T) {
	Convey("Given a ticker touch and one executed trade", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		at := time.Unix(1_700_002_300, 0).UTC()

		market.AppendTicker(kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromInt64(99),
			Ask:       decimal.NewFromInt64(101),
			Timestamp: at,
		})
		market.AppendTrade(kraken.TradeData{
			Symbol:    "BTC/USD",
			Side:      "buy",
			Price:     *decimal.NewFromInt64(100),
			Qty:       2,
			TradeID:   1,
			Timestamp: at.Add(time.Second),
		})

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should emit ignition evidence for the print", func() {
			measurements := []*nmtypes.Measurement{}

			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, string(types.SourcePumpDump))
		})
	})
}
