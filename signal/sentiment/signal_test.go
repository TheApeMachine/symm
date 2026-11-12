package sentiment_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/signal/sentiment"
)

func TestSentimentStep(t *testing.T) {
	Convey("Step records sentiment once conviction peers exist", t, func() {
		signal := sentiment.NewSignal(context.Background())
		envelope := runtime.NewEnvelope(nil)
		at := time.Now().UTC()
		symbols := []string{"S1/USD", "S2/USD", "S3/USD", "S4/USD"}

		for i := range 24 {
			trades := make([]*market.TradeData, 0, len(symbols))
			for j, symbol := range symbols {
				side := market.SideBuy
				if i%3 == 0 {
					side = market.SideSell
				}

				trades = append(trades, &market.TradeData{
					Symbol:    symbol,
					Side:      side,
					Price:     decimal.NewFromFloat(12 + float64(i)*0.1 + float64(j)),
					Qty:       decimal.NewFromInt(3),
					Timestamp: at.Add(time.Duration(i) * time.Second),
				})
			}

			envelope.Trade = &market.Trade{Data: trades}
			envelope = signal.Step(envelope)
		}

		reading := envelope.Sentiment["S1/USD"]
		So(reading, ShouldNotBeNil)
		So(int(reading.Category), ShouldEqual, 1)
		So(reading.SurgeScore, ShouldEqual, 1)
		So(reading.Breadth, ShouldEqual, 1)
		So(reading.SlumpScore, ShouldEqual, 0)
		So(reading.DivergentScore, ShouldEqual, 0)
		So(reading.Value, ShouldEqual, 1)
	})
}
