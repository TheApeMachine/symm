package market

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
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
			convey.So(instrument.subscribers["raw"], convey.ShouldNotBeNil)
			convey.So(instrument.subscribers["raw"].ID, convey.ShouldEqual, instrumentSubscriberID)
		})
	})
}

func TestInstrumentApplyCatalogUpdate(t *testing.T) {
	convey.Convey("Given a filtered instrument catalog update", t, func() {
		viper.Set("market.symbols", []string{"BTC/EUR"})
		viper.Set("market.max_scan_symbols", 8)

		outbound, outboundErr := qpool.NewBroadcastGroup(
			context.Background(), "test:instrument:outbound", 10*time.Millisecond,
		)

		convey.So(outboundErr, convey.ShouldBeNil)

		subscriber := outbound.Subscribe("test:instrument:outbound", 16)

		instrument := &Instrument{Pairs: make([]string, 0)}

		data, marshalErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "ETH/EUR", Base: "ETH", Quote: "EUR"},
				{Symbol: "BTC/EUR", Base: "BTC", Quote: "EUR"},
			},
		})

		convey.So(marshalErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, public.SocketMessage{
			Channel: public.InstrumentsChannel,
			Type:    "snapshot",
			Data:    data,
		})

		convey.Convey("It should subscribe only configured symbols", func() {
			convey.So(instrument.Pairs, convey.ShouldResemble, []string{"BTC/EUR"})

			frame := <-subscriber.Incoming
			convey.So(frame, convey.ShouldNotBeNil)
			convey.So(frame.Value, convey.ShouldNotBeNil)
		})
	})
}
