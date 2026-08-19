package correlation

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

func TestCorrelationNumber(t *testing.T) {
	Convey("Given tickers on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()

		for index := 0; index < 4; index++ {
			market.AppendTicker(correlationTicker(
				"AAA/USD", 100+float64(index)*10, start.Add(time.Duration(index)*time.Second),
			))
		}

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should emit conditioned correlation evidence per tick", func() {
			measurements := []*nmtypes.Measurement{}

			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 4)

			for _, measurement := range measurements {
				So(measurement.Source, ShouldEqual, string(types.SourceCorrelation))
				So(measurement.Symbol, ShouldEqual, "AAA/USD")
			}
		})
	})
}

func correlationTicker(symbol string, price float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      decimal.NewFromFloat64(price),
		Change:    decimal.NewFromFloat64(price),
		Timestamp: at,
	}
}
