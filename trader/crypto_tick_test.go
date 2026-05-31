package trader

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/wallet"
)

func TestCryptoTickReturnsOnCancel(t *testing.T) {
	convey.Convey("Given a running crypto desk", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		crypto := &Crypto{
			ctx:      ctx,
			cancel:   cancel,
			wallet:   wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26),
			tracker:  focus.NewSet(),
			quotes:   newQuoteCache(),
			paper:    NewPaperSession(ctx),
			makers:   newMakerDesk(),
			readings: make(map[string]map[perspectives.SourceType]timedMeasurement),
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
