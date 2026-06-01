package market

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestNewInstrument(t *testing.T) {
	convey.Convey("Given a parent context and pool", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)

		convey.Convey("It should publish outbound frames without owning kraken:public", func() {
			instrument := NewInstrument(ctx, pool)

			convey.So(instrument.broadcasts["kraken:public"], convey.ShouldNotBeNil)
			convey.So(instrument.subscribers["kraken:public"], convey.ShouldBeNil)
			convey.So(instrument.subscribers["instrument"], convey.ShouldNotBeNil)
			convey.So(instrument.subscribers["instrument"].ID, convey.ShouldEqual, instrumentSubscriberID)
		})
	})
}
