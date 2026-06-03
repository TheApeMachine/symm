package replay

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestNewTrader(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())

		pool := qpool.NewQ(ctx, 1, 4, nil)
		trader, err := NewTrader(ctx, pool)

		So(err, ShouldBeNil)

		Convey("It should exit Tick when the context is cancelled", func() {
			done := make(chan error, 1)

			go func() {
				done <- trader.Tick()
			}()

			cancel()

			select {
			case tickErr := <-done:
				So(errors.Is(tickErr, context.Canceled), ShouldBeTrue)
			case <-time.After(time.Second):
				So("Tick exit", ShouldBeBlank)
			}

			So(trader.Close(), ShouldBeNil)
		})
	})
}
