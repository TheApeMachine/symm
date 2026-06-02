package optimizer

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestNewTrader(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		trader, err := NewTrader(ctx, pool)

		So(err, ShouldBeNil)

		Convey("It should tick without error", func() {
			So(trader.Tick(), ShouldBeNil)
			So(trader.Close(), ShouldBeNil)
		})
	})
}
