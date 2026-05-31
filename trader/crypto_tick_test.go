package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/wallet"
)

func TestCryptoTickReturnsOnCancel(t *testing.T) {
	convey.Convey("Given a running crypto desk", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ(ctx, 1, 4, qpool.NewConfig())
		group := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

		crypto := &Crypto{
			ctx:          ctx,
			cancel:       cancel,
			wallet:       wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26),
			tracker:      focus.NewSet(),
			quotes:       newQuoteCache(),
			paper:        NewPaperSession(ctx),
			makers:       newMakerDesk(),
			readings:     make(map[string]map[perspectives.SourceType]timedMeasurement),
			measurements: group.Subscribe("test:crypto-tick", 8),
		}
		done := make(chan error, 1)

		go func() {
			done <- crypto.Tick()
		}()

		time.Sleep(10 * time.Millisecond)
		cancel()
		_ = crypto.Close()

		var tickErr error

		select {
		case tickErr = <-done:
		case <-time.After(time.Second):
			t.Fatal("Tick did not return after cancel")
		}

		convey.Convey("It should exit when the context is cancelled", func() {
			convey.So(tickErr, convey.ShouldNotBeNil)
		})
	})
}

func TestMakerDeskDropPreservesNewerEntry(t *testing.T) {
	convey.Convey("Given a symbol remapped to a newer clOrdID", t, func() {
		desk := newMakerDesk()
		desk.track(&restingMakerEntry{clOrdID: "old", symbol: "BTC/EUR"})
		desk.track(&restingMakerEntry{clOrdID: "new", symbol: "BTC/EUR"})

		desk.drop("old", "BTC/EUR")

		convey.Convey("It should keep the newer entry indexed", func() {
			convey.So(desk.HasPending("BTC/EUR"), convey.ShouldBeTrue)
			entry, ok := desk.entryFor("new")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(entry.clOrdID, convey.ShouldEqual, "new")
		})
	})
}
