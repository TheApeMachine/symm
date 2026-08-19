package leadlag

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

func TestLeadLagNumber(t *testing.T) {
	Convey("Given repeated ticks on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_007_000, 0).UTC()

		for leg, price := range []float64{100, 110, 121, 133} {
			market.AppendTicker(kraken.TickerData{
				Symbol:    "AAA/USD",
				Last:      decimal.NewFromFloat64(price),
				Timestamp: start.Add(time.Duration(leg) * time.Second),
			})
		}

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("It should emit velocity-conditioned lag evidence per tick", func() {
			measurements := []*nmtypes.Measurement{}

			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 4)
			So(measurements[0].Source, ShouldEqual, string(types.SourceLeadLag))
		})
	})
}
