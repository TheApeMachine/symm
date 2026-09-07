package strategy

import (
	"context"
	"testing"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	spot "github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/venue"
	"github.com/theapemachine/symm/types"
)

func virtualFixture() (virtualWallet, *spotbook.Book) {
	instrument := broker.NewInstrumentWithQuote("USD")
	instrument.Cache([]kraken.InstrumentPair{{
		Symbol:       "TEST/USD",
		Base:         "TEST",
		Quote:        "USD",
		Status:       "online",
		QtyIncrement: decimal.NewFromInt64(1),
		QtyMin:       decimal.NewFromInt64(1),
		CostMin:      decimal.NewFromInt64(1),
	}})
	conn := venue.NewConn()
	api := websocket.NewAPI(context.Background(), conn, conn)
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"TEST": {AltName: "TEST", Decimals: 8, DisplayDecimals: 8},
			"USD":  {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
		},
		NewPairs: map[string]spot.AssetPair{
			"TESTUSD": {
				WSName: "TEST/USD", Base: "TEST", Quote: "USD",
				PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1,
			},
		},
	})
	price := broker.NewPrice(api, instrument)
	price.SetFee("TEST/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromInt64(1)})

	wallet := virtualWallet{}
	err := wallet.initialize(decimal.NewFromInt64(1000), price, "TEST/USD")
	if err != nil {
		panic(err)
	}
	book := spotbook.New()
	book.NoBookCrossing = false

	for _, level := range []struct {
		direction       spotbook.BookDirection
		id              string
		price, quantity int64
	}{
		{spotbook.Bid, "bid", 100, 10}, {spotbook.Ask, "ask", 101, 3}, {spotbook.Ask, "deep", 102, 10},
	} {
		book.Update(&spotbook.UpdateOptions{Direction: level.direction, ID: level.id,
			Price: decimal.NewFromInt64(level.price), Quantity: decimal.NewFromInt64(level.quantity), Silent: true})
	}
	return wallet, book
}

func TestVirtualWalletFill(t *testing.T) {
	Convey("Given an independent wallet and executable ask levels", t, func() {
		wallet, book := virtualFixture()
		enter := LearningAction{Kind: types.ActionEnter}
		requested := decimal.NewFromInt64(2)
		quantity, gross, fee, err := wallet.fill(book, enter, requested)

		So(err, ShouldBeNil)
		So(quantity.Cmp(requested), ShouldEqual, 0)
		So(gross.Sign(), ShouldBeGreaterThan, 0)
		So(fee.Sign(), ShouldBeGreaterThan, 0)
		So(wallet.cash.Cmp(decimal.NewFromInt64(1000)), ShouldBeLessThan, 0)

		Convey("Liquidation marks reflect visible bids and fees", func() {
			mark, complete, markErr := wallet.mark(book)
			So(markErr, ShouldBeNil)
			So(complete, ShouldBeTrue)
			So(mark.Sign(), ShouldBeGreaterThan, 0)
		})

		Convey("A wait changes neither the account nor its fees", func() {
			beforeCash := wallet.cash
			q, g, f, waitErr := wallet.fill(book, LearningAction{Kind: types.ActionHold}, decimal.NewFromInt64(0))
			So(waitErr, ShouldBeNil)
			So(q.Sign(), ShouldEqual, 0)
			So(g.Sign(), ShouldEqual, 0)
			So(f.Sign(), ShouldEqual, 0)
			So(wallet.cash.Cmp(beforeCash), ShouldEqual, 0)
		})
	})
}

func TestVirtualWalletRestart(t *testing.T) {
	Convey("Given a wallet that has accumulated fees", t, func() {
		wallet, book := virtualFixture()
		_, _, _, err := wallet.fill(book, LearningAction{Kind: types.ActionEnter}, decimal.NewFromInt64(1))
		So(err, ShouldBeNil)
		So(wallet.fees.Sign(), ShouldBeGreaterThan, 0)

		Convey("Restart resets cash and returns spent fees", func() {
			spent := wallet.restart(decimal.NewFromInt64(500))
			So(spent.Sign(), ShouldBeGreaterThan, 0)
			So(wallet.cash.Cmp(decimal.NewFromInt64(500)), ShouldEqual, 0)
			So(wallet.fees.Sign(), ShouldEqual, 0)
			So(wallet.quantity.Sign(), ShouldEqual, 0)
		})
	})
}
