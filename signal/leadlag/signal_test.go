package leadlag

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given repeated multi-symbol lead-lag cuts", t, func() {
		signal := &Signal{ctx: context.Background(), section: NewSection()}
		start := time.Unix(1_700_007_000, 0).UTC()

		Reset(func() {
			signal.Close()
		})

		for leg, prices := range [][]float64{
			{100, 100, 100},
			{110, 101, 99},
			{121, 102, 98},
		} {
			at := start.Add(time.Duration(leg) * time.Second)
			thesis := types.NewThesis(t.Context(), nil)

			for index, symbol := range []string{"AAA/USD", "BBB/USD", "CCC/USD"} {
				thesis.Tickers.Store(symbol, []kraken.TickerData{{
					Symbol:    symbol,
					Last:      decimal.NewFromFloat64(prices[index]),
					Timestamp: at,
				}})
			}

			measurements := signal.Measure(thesis)

			if leg == 0 {
				So(measurements, ShouldHaveLength, 3)
			}

			if leg == 2 {
				So(measurements, ShouldHaveLength, 3)
			}
		}

		Convey("It retains every symbol history across measurement passes", func() {
			for _, symbol := range []string{"AAA/USD", "BBB/USD", "CCC/USD"} {
				So(signal.section.PriceSampleCount(symbol), ShouldEqual, 3)
			}
		})
	})
}
