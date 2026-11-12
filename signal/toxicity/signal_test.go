package toxicity_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/toxicity"
)

func pushBook(books *broker.BookManager, symbol string, bidQty int64) {
	bidPx := decimal.NewFromInt(90)
	bQty := decimal.NewFromInt(bidQty)
	askPx := decimal.NewFromInt(91)
	aQty := decimal.NewFromInt(80)

	books.Update(&market.Level3{
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				LimitPrice: &bidPx,
				OrderQty:   &bQty,
			}},
			Asks: []*market.Level3Order{{
				LimitPrice: &askPx,
				OrderQty:   &aQty,
			}},
		}},
	})
}

func TestToxicityStep(t *testing.T) {
	Convey("Step records hard support after touch refills", t, func() {
		symbol := "TOX/USD"
		books := broker.NewBookManager()

		pushBook(books, symbol, 20)

		signal := toxicity.NewSignal(context.Background(), books)
		envelope := runtime.NewEnvelope(nil)
		at := time.Now().UTC()

		var out *runtime.Envelope
		for i := range 48 {
			pushBook(books, symbol, 20+int64(i)*5)

			envelope.Trade = &market.Trade{Data: []*market.TradeData{{
				Symbol:    symbol,
				Side:      market.SideBuy,
				Price:     decimal.NewFromFloat(90.5),
				Qty:       decimal.NewFromInt(2),
				Timestamp: at.Add(time.Duration(i) * time.Second),
			}}}
			out = signal.Step(envelope)
		}

		reading := out.Toxicity[symbol]
		So(reading, ShouldNotBeNil)
		So(int(reading.Category), ShouldEqual, 3)
		So(reading.BluffScore, ShouldEqual, 0)
		So(reading.VacuumScore, ShouldEqual, 0)
		So(reading.SupportScore, ShouldBeGreaterThan, 0.3)
		So(reading.Value, ShouldBeGreaterThan, 0.25)
	})
}
