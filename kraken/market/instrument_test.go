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
	convey.Convey("Given an instrument catalog update", t, func() {
		defer viper.Reset()

		viper.Set("market.max_scan_symbols", 0)
		viper.Set("market.quote_currency", "EUR")

		outbound, outboundErr := qpool.NewBroadcastGroup(
			context.Background(), "test:instrument:outbound", 10*time.Millisecond,
		)

		convey.So(outboundErr, convey.ShouldBeNil)

		subscriber := outbound.Subscribe("test:instrument:outbound", 16)

		instrument := &Instrument{
			Pairs:   make([]string, 0),
			pairSet: make(map[string]struct{}),
		}

		data, marshalErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "ETH/EUR", Base: "ETH", Quote: "EUR"},
				{Symbol: "BTC/USD", Base: "BTC", Quote: "USD"},
				{Symbol: "BTC/EUR", Base: "BTC", Quote: "EUR"},
			},
		})

		convey.So(marshalErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, map[string]any{
			"channel": public.InstrumentsChannel,
			"type":    "snapshot",
			"data":    data,
		})

		convey.Convey("It should subscribe only quote-currency pairs", func() {
			convey.So(instrument.Pairs, convey.ShouldResemble, []string{"ETH/EUR", "BTC/EUR"})

			var frame *qpool.QValue[any]

			select {
			case frame = <-subscriber.Incoming:
			case <-time.After(2 * time.Second):
				convey.So("applyCatalogUpdate did not emit a subscribe frame", convey.ShouldBeEmpty)
			}

			convey.So(frame, convey.ShouldNotBeNil)
			convey.So(frame.Value, convey.ShouldNotBeNil)
		})
	})

	convey.Convey("Given an explicit market.symbols watchlist", t, func() {
		defer viper.Reset()

		viper.Set("market.symbols", []string{"BTC/EUR"})

		outbound, outboundErr := qpool.NewBroadcastGroup(
			context.Background(), "test:instrument:outbound-watch", 10*time.Millisecond,
		)

		convey.So(outboundErr, convey.ShouldBeNil)

		instrument := &Instrument{
			Pairs:   make([]string, 0),
			pairSet: make(map[string]struct{}),
		}

		data, marshalErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "ETH/EUR", Base: "ETH", Quote: "EUR"},
				{Symbol: "BTC/EUR", Base: "BTC", Quote: "EUR"},
			},
		})

		convey.So(marshalErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, map[string]any{
			"channel": public.InstrumentsChannel,
			"type":    "snapshot",
			"data":    data,
		})

		convey.So(instrument.Pairs, convey.ShouldResemble, []string{"BTC/EUR"})
	})

	convey.Convey("Given book_depth_levels in config", t, func() {
		defer viper.Reset()

		viper.Set("market.quote_currency", "EUR")
		viper.Set("market.book_depth_levels", 10)

		outbound, outboundErr := qpool.NewBroadcastGroup(
			context.Background(), "test:instrument:outbound-depth", 10*time.Millisecond,
		)

		convey.So(outboundErr, convey.ShouldBeNil)

		subscriber := outbound.Subscribe("test:instrument:outbound-depth", 16)

		instrument := &Instrument{
			Pairs:   make([]string, 0),
			pairSet: make(map[string]struct{}),
		}

		data, marshalErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "BTC/EUR", Base: "BTC", Quote: "EUR"},
			},
		})

		convey.So(marshalErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, map[string]any{
			"channel": public.InstrumentsChannel,
			"type":    "snapshot",
			"data":    data,
		})

		var bookFrame *qpool.QValue[any]

		for range 3 {
			select {
			case frame := <-subscriber.Incoming:
				payload, ok := frame.Value.(map[string]any)

				if !ok {
					continue
				}

				params, ok := payload["params"].(map[string]any)

				if !ok {
					continue
				}

				if params["channel"] == "book" {
					bookFrame = frame
				}
			case <-time.After(2 * time.Second):
			}
		}

		convey.So(bookFrame, convey.ShouldNotBeNil)

		params, ok := bookFrame.Value.(map[string]any)["params"].(map[string]any)

		convey.So(ok, convey.ShouldBeTrue)
		convey.So(params["depth"], convey.ShouldEqual, 10)
	})

	convey.Convey("Given delisted pairs in the catalog", t, func() {
		defer viper.Reset()

		viper.Set("market.quote_currency", "EUR")

		outbound, outboundErr := qpool.NewBroadcastGroup(
			context.Background(), "test:instrument:outbound-status", 10*time.Millisecond,
		)

		convey.So(outboundErr, convey.ShouldBeNil)

		instrument := &Instrument{
			Pairs:   make([]string, 0),
			pairSet: make(map[string]struct{}),
		}

		data, marshalErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "DEAD/EUR", Base: "DEAD", Quote: "EUR", Status: "delisted"},
				{Symbol: "PEPE/EUR", Base: "PEPE", Quote: "EUR", Status: "online"},
			},
		})

		convey.So(marshalErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, map[string]any{
			"channel": public.InstrumentsChannel,
			"type":    "snapshot",
			"data":    data,
		})

		convey.So(instrument.Pairs, convey.ShouldResemble, []string{"PEPE/EUR"})
	})

	convey.Convey("Given max_scan_symbols caps discovery", t, func() {
		defer viper.Reset()

		viper.Set("market.max_scan_symbols", 1)
		viper.Set("market.quote_currency", "EUR")

		outbound, outboundErr := qpool.NewBroadcastGroup(
			context.Background(), "test:instrument:outbound-cap", 10*time.Millisecond,
		)

		convey.So(outboundErr, convey.ShouldBeNil)

		instrument := &Instrument{
			Pairs:   make([]string, 0),
			pairSet: make(map[string]struct{}),
		}

		data, marshalErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "ETH/EUR", Base: "ETH", Quote: "EUR"},
				{Symbol: "BTC/EUR", Base: "BTC", Quote: "EUR"},
			},
		})

		convey.So(marshalErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, map[string]any{
			"channel": public.InstrumentsChannel,
			"type":    "snapshot",
			"data":    data,
		})

		convey.So(len(instrument.Pairs), convey.ShouldEqual, 1)
	})

	convey.Convey("Given a newly listed pair on a later catalog update", t, func() {
		defer viper.Reset()

		viper.Set("market.max_scan_symbols", 0)
		viper.Set("market.quote_currency", "EUR")

		outbound, outboundErr := qpool.NewBroadcastGroup(
			context.Background(), "test:instrument:outbound-new", 10*time.Millisecond,
		)

		convey.So(outboundErr, convey.ShouldBeNil)

		instrument := &Instrument{
			Pairs:   make([]string, 0),
			pairSet: make(map[string]struct{}),
		}

		first, firstErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "BTC/EUR", Base: "BTC", Quote: "EUR"},
			},
		})

		convey.So(firstErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, map[string]any{
			"channel": public.InstrumentsChannel,
			"type":    "snapshot",
			"data":    first,
		})

		second, secondErr := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{Symbol: "PEPE/EUR", Base: "PEPE", Quote: "EUR"},
			},
		})

		convey.So(secondErr, convey.ShouldBeNil)

		instrument.applyCatalogUpdate(outbound, map[string]any{
			"channel": public.InstrumentsChannel,
			"type":    "update",
			"data":    second,
		})

		convey.So(instrument.Pairs, convey.ShouldResemble, []string{"BTC/EUR", "PEPE/EUR"})
	})
}
