package broker

import (
	"context"
	"sync"
	"testing"

	sdkbook "github.com/krakenfx/api-go/v2/pkg/book"
	sdkdecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	shop "github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
)

func TestAllocateUsesWallet(t *testing.T) {
	Convey("Allocate sizes from the passed wallet, and nil uses the desk balance", t, func() {
		const symbol = "BTC/USD"

		paper := market.NewBalance("USD")
		paper.Seed(shop.NewFromInt(100000))

		real := market.NewBalance("USD")
		real.Seed(shop.NewFromInt(500))

		qtyMin, err := decimal.NewFromString("0.00000001")
		So(err, ShouldBeNil)
		costMin, err := decimal.NewFromString("0.01")
		So(err, ShouldBeNil)
		step, err := decimal.NewFromString("0.00000001")
		So(err, ShouldBeNil)

		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store(symbol, kraken.InstrumentPair{
			Symbol:         symbol,
			Quote:          "USD",
			QtyPrecision:   8,
			PricePrecision: 2,
			QtyMin:         qtyMin,
			CostMin:        costMin,
			QtyIncrement:   step,
		})

		books := NewBook(context.Background(), spot.NewNormalizer())
		So(books.Create(symbol, 10), ShouldBeNil)

		managed := books.manager.GetBook(symbol)
		So(managed, ShouldNotBeNil)

		managed.Update(&sdkbook.UpdateOptions{
			Direction: sdkbook.Bid,
			Price:     sdkdecimal.NewFromInt64(99),
			Quantity:  sdkdecimal.NewFromInt64(1000000),
		})
		managed.Update(&sdkbook.UpdateOptions{
			Direction: sdkbook.Ask,
			Price:     sdkdecimal.NewFromInt64(100),
			Quantity:  sdkdecimal.NewFromInt64(1000000),
		})

		price := &Price{
			Instrument: instrument,
			Books:      books,
			balance:    paper,
			normalizer: spot.NewNormalizer(),
		}

		fromDesk, deskWhy := price.Allocate(symbol, nil)
		fromPaper, paperWhy := price.Allocate(symbol, paper)
		fromReal, realWhy := price.Allocate(symbol, real)

		So(deskWhy, ShouldEqual, "")
		So(paperWhy, ShouldEqual, "")
		So(realWhy, ShouldEqual, "")
		So(fromDesk, ShouldNotBeNil)
		So(fromPaper, ShouldNotBeNil)
		So(fromReal, ShouldNotBeNil)
		So(fromDesk.Cmp(fromPaper), ShouldEqual, 0)
		So(fromReal.Cmp(fromPaper), ShouldEqual, -1)

		position := NewPosition(
			context.Background(), symbol, "real", "path", price, nil, nil,
		)
		So(position.Enter("path", nil, real), ShouldBeNil)
		So(position.EntryOrder.Volume, ShouldEqual, fromReal.String())
	})
}
