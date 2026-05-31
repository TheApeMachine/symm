package broker

import (
	"testing"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

func TestQueueAheadBaseQty(t *testing.T) {
	Convey("Given bid depth at the limit price", t, func() {
		bids := []market.BookLevel{
			{Price: 50100, Qty: 0.5},
			{Price: 50000, Qty: 1.2},
			{Price: 49900, Qty: 3},
		}

		ahead := QueueAheadBaseQty(bids, 50000)

		Convey("It should sum visible quantity at the limit", func() {
			So(ahead, ShouldEqual, 1.2)
		})
	})
}

func TestMakerFillReady(t *testing.T) {
	Convey("Given insufficient sell-aggressor volume", t, func() {
		queue := MakerQueueContext{
			Bids: []market.BookLevel{
				{Price: 50000, Qty: 1},
			},
			BidTradeVolume: 0.5,
		}

		ready := MakerFillReady(queue, 50000, 0.1)

		Convey("It should not allow a fill", func() {
			So(ready, ShouldBeFalse)
		})
	})

	Convey("Given sell-aggressor volume through the queue", t, func() {
		queue := MakerQueueContext{
			Bids: []market.BookLevel{
				{Price: 50000, Qty: 1},
			},
			BidTradeVolume: 1.15,
		}

		ready := MakerFillReady(queue, 50000, 0.1)

		Convey("It should allow a fill", func() {
			So(ready, ShouldBeTrue)
		})
	})
}

func TestMakerFillPaperRequiresQueue(t *testing.T) {
	Convey("Given a maker bid with insufficient queue turnover", t, func() {
		tradingWallet := walletForMakerTest(t, 10)

		fill, err := (&Maker{
			Symbol:     "BTC/EUR",
			LimitPrice: 50000,
			Notional:   10,
		}).FillPaper(tradingWallet, MakerQueueContext{
			Bids: []market.BookLevel{
				{Price: 50000, Qty: 2},
			},
			BidTradeVolume: 0.1,
		})

		Convey("It should reject without settling", func() {
			So(err, ShouldEqual, ErrMakerQueueNotReady)
			So(fill.Qty, ShouldEqual, 0)
			So(tradingWallet.ReservedEUR, ShouldEqual, 10)
		})
	})
}

func walletForMakerTest(t *testing.T, notional float64) *wallet.Wallet {
	t.Helper()

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)

	if err := tradingWallet.ReserveEntry(notional); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	return tradingWallet
}

func BenchmarkMakerFillReady(b *testing.B) {
	queue := MakerQueueContext{
		Bids: []market.BookLevel{
			{Price: 50000, Qty: 1.5},
			{Price: 49950, Qty: 2},
		},
		BidTradeVolume: 2,
	}

	b.ReportAllocs()

	for b.Loop() {
		if !MakerFillReady(queue, 50000, 0.25) {
			b.Fatal("expected ready")
		}
	}
}
