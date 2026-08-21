package pumpdump

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestSignalConsumeTicker(t *testing.T) {
	Convey("Given ticker rows for one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		symbol := thesis.Symbol("BTC/USD")
		signal := NewSignal(context.Background(), thesis, nil)
		at := time.Unix(1_700_002_300, 0).UTC()
		defer signal.Close()

		Convey("A zero last is an absent observation and does not poison state", func() {
			zero := decimal.NewFromInt64(0)
			err := signal.consumeTicker(symbol, kraken.TickerData{
				Symbol:    symbol.Symbol,
				Last:      zero,
				Timestamp: at,
			})

			So(err, ShouldBeNil)
			So(drainMeasurements(symbol), ShouldBeEmpty)

			for offset, value := range []int64{100, 110, 121} {
				last := decimal.NewFromInt64(value)
				err = signal.consumeTicker(symbol, kraken.TickerData{
					Symbol:    symbol.Symbol,
					Last:      last,
					Timestamp: at.Add(time.Duration(offset+1) * time.Second),
				})
				So(err, ShouldBeNil)
			}

			measurements := drainMeasurements(symbol)
			So(len(measurements), ShouldEqual, 3)
			precursor := measurements[2].Metrics[types.MetricKey(
				types.MetricPrecursor,
				types.SideBuy,
			)]
			So(precursor.Raw, ShouldAlmostEqual, math.Log(1.1), 1e-15)
			So(precursor.Normalized, ShouldNotBeNil)
			So(*precursor.Normalized, ShouldAlmostEqual, 0.5, 1e-15)
			So(measurements[2].ObservedFrom, ShouldResemble,
				measurements[2].At)
		})

		Convey("A missing last remains a visible error", func() {
			err := signal.consumeTicker(symbol, kraken.TickerData{
				Symbol:    symbol.Symbol,
				Timestamp: at,
			})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				"pumpdump: ticker requires a last price")
		})
	})
}

func drainMeasurements(symbol *types.Symbol) []*nmtypes.Measurement {
	measurements := []*nmtypes.Measurement{}

	for measurement := range symbol.MarketMeasurements(
		symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
	) {
		measurements = append(measurements, measurement)
	}

	return measurements
}
