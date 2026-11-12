package cvd_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/cvd"
)

func seedBook(books *broker.BookManager, symbol string, bid, ask int64) {
	bidPx := decimal.NewFromInt(bid)
	bidQty := decimal.NewFromInt(10)
	askPx := decimal.NewFromInt(ask)
	askQty := decimal.NewFromInt(10)

	books.Update(&market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				LimitPrice: &bidPx,
				OrderQty:   &bidQty,
			}},
			Asks: []*market.Level3Order{{
				LimitPrice: &askPx,
				OrderQty:   &askQty,
			}},
		}},
	})
}

func TestCVDStep(t *testing.T) {
	Convey("aggressive buy flow with rising response classifies as drive", t, func() {
		symbol := "CVD/USD"
		books := broker.NewBookManager()
		signal := cvd.NewSignal(context.Background(), books)
		envelope := runtime.NewEnvelope(nil)
		at := time.Now().UTC()

		var out *runtime.Envelope
		for i := range 16 {
			seedBook(books, symbol, 100+int64(i), 102+int64(i))

			price := decimal.NewFromInt(103 + int64(i))
			envelope.Trade = &market.Trade{Data: []*market.TradeData{{
				Symbol:    symbol,
				Side:      market.SideBuy,
				Price:     price,
				Qty:       decimal.NewFromInt(2),
				Timestamp: at.Add(time.Duration(i) * time.Second),
			}}}

			out = signal.Step(envelope)
		}

		reading := out.CVD[symbol]
		So(reading, ShouldNotBeNil)
		So(int(reading.Category), ShouldEqual, 2)
		So(reading.Drive, ShouldEqual, 1)
		So(reading.Absorption, ShouldEqual, 0)
		So(reading.NetFraction, ShouldEqual, 1)
		So(reading.Net, ShouldEqual, 932)
		So(reading.Value, ShouldEqual, 1)
	})
}
