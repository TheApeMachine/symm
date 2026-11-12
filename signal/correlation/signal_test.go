package correlation_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/signal/correlation"
)

func TestCorrelationStep(t *testing.T) {
	Convey("Step attaches cohort readings once a peer cohort exists", t, func() {
		signal := correlation.NewSignal(context.Background())
		envelope := runtime.NewEnvelope(nil)
		at := time.Now().UTC()

		// Peers land before the focus symbol so its cohort sees one shared price.
		order := []string{"B/USD", "C/USD", "D/USD", "E/USD", "A/USD"}

		for i := range 128 {
			price := decimal.NewFromFloat(100 + float64(i))
			ts := at.Add(time.Duration(i) * time.Second)

			for _, symbol := range order {
				envelope.Trade = &market.Trade{Data: []*market.TradeData{{
					Symbol:    symbol,
					Side:      market.SideBuy,
					Price:     price,
					Qty:       decimal.NewFromInt(2),
					Timestamp: ts,
				}}}
				envelope = signal.Step(envelope)
			}
		}

		reading := envelope.Correlation["A/USD"]
		So(reading, ShouldNotBeNil)
		So(reading.Eligible, ShouldBeTrue)
		So(reading.Category, ShouldEqual, 1)
		So(reading.HerdScore, ShouldEqual, 1)
		So(reading.AlphaScore, ShouldEqual, 0)
		So(reading.NoiseScore, ShouldEqual, 0)
		So(reading.Correlation, ShouldEqual, 1)
		So(reading.Energy, ShouldBeGreaterThan, 0.004)
		So(reading.Energy, ShouldBeLessThan, 0.005)
		So(reading.Strength, ShouldEqual, 1)
	})
}
