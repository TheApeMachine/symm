package cvd

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

func cvdTrade(
	tradeID int64,
	side string,
	price float64,
	qty float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    "BTC/USD",
		TradeID:   tradeID,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       qty,
		Timestamp: at,
	}
}

func TestCVDNumber(t *testing.T) {
	Convey("Given a sequence of buy then sell executions on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		start := time.Unix(1_700_003_200, 0).UTC()

		market.AppendTrade(cvdTrade(1, "buy", 100.0, 2, start))
		market.AppendTrade(cvdTrade(2, "sell", 100.1, 1, start.Add(time.Second)))

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()
		go signal.Run()

		Convey("It should emit flow decomposition from the nomagique pipeline", func() {
			measurements := []*nmtypes.Measurement{}

			deadline := time.Now().Add(2 * time.Second)

			for len(measurements) < 2 && time.Now().Before(deadline) {
				measurements = measurements[:0]

				for measurement := range market.MarketMeasurements("category") {
					measurements = append(measurements, measurement)
				}

				time.Sleep(time.Millisecond)
			}

			So(len(measurements), ShouldEqual, 2)

			for _, measurement := range measurements {
				_, hasNet := measurement.Metrics["net"]
				So(hasNet, ShouldBeTrue)
				So(measurement.Metrics["absorption"].Normalized, ShouldNotBeNil)
			}
		})
	})
}
