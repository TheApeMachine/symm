package sentiment

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

func TestSentimentNumber(t *testing.T) {
	Convey("Given ticks on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()

		for index, price := range []float64{100, 102, 101, 105} {
			market.AppendTicker(kraken.TickerData{
				Symbol:    "AAA/USD",
				Last:      decimal.NewFromFloat64(price),
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
		}

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should emit a sentiment deviation reading per tick", func() {
			measurements := []*nmtypes.Measurement{}

			time.Sleep(50 * time.Millisecond)
			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 4)
			So(measurements[0].Source, ShouldEqual, string(types.SourceSentiment))
		})
	})
}
