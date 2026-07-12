package trader

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestInstrumentOn(t *testing.T) {
	Convey("Given a USD instrument cache", t, func() {
		instrument := NewInstrument(nil, nil, nil)
		instrument.quote = "USD"
		payload, err := sonic.Marshal(kraken.InstrumentData{Pairs: []kraken.InstrumentPair{
			{Symbol: "BTC/USD", Quote: "USD", Status: "online"},
			{Symbol: "ETH/EUR", Quote: "EUR", Status: "online"},
			{Symbol: "OLD/USD", Quote: "USD", Status: "cancel_only"},
		}})
		So(err, ShouldBeNil)

		Convey("When mixed quote and status rows arrive", func() {
			instrument.On(payload)

			Convey("Then only online instruments in the configured quote are retained", func() {
				pairs := instrument.Pairs()
				So(pairs, ShouldHaveLength, 1)
				So(pairs[0].Symbol, ShouldEqual, "BTC/USD")
				So(pairs[0].Quote, ShouldEqual, "USD")
				So(pairs[0].Status, ShouldEqual, "online")
			})
		})
	})
}

func TestInstrumentSubscribe(t *testing.T) {
	Convey("Given an online USD universe and staged ticker snapshots", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe_batch", 2)
		viper.Set("market.subscribe_pace", 0)
		viper.Set("market.universe.trading_tier_size", 1)
		viper.Set("market.l3_enabled", false)

		priceAPI := &instrumentPriceAPI{}
		price := broker.NewPrice(priceAPI, nil)
		subscriptions := &instrumentAPIStub{}
		instrument := NewInstrument(subscriptions, price, nil)
		payload, err := sonic.Marshal(kraken.InstrumentData{Pairs: []kraken.InstrumentPair{
			{Symbol: "SOL/USD", Quote: "USD", Status: "online"},
			{Symbol: "BTC/USD", Quote: "USD", Status: "online"},
			{Symbol: "ETH/USD", Quote: "USD", Status: "online"},
		}})
		So(err, ShouldBeNil)
		instrument.On(payload)

		Convey("When observation starts before ticker coverage is complete", func() {
			err := instrument.Subscribe()
			So(err, ShouldBeNil)
			So(instrument.Status(), ShouldEqual, types.PENDING)
			So(subscriptions.calls["ticker"], ShouldResemble, [][]string{
				{"BTC/USD", "ETH/USD"},
				{"SOL/USD"},
			})
			So(subscriptions.calls["ohlc"], ShouldResemble, [][]string{
				{"BTC/USD", "ETH/USD"},
				{"SOL/USD"},
			})
			So(subscriptions.calls["trade"], ShouldBeEmpty)
			So(priceAPI.requested, ShouldBeEmpty)

			price.TickerAck([]byte(`{
				"channel":"ticker",
				"type":"snapshot",
				"data":[
					{"symbol":"BTC/USD","bid":"90","bid_qty":10,"ask":"110","ask_qty":10,"last":"100","volume":10,"vwap":100},
					{"symbol":"ETH/USD","bid":"99","bid_qty":5,"ask":"101","ask_qty":5,"last":"100","volume":6,"vwap":100}
				]
			}`))

			err = instrument.Subscribe()
			So(err, ShouldBeNil)
			So(instrument.Status(), ShouldEqual, types.PENDING)
			So(subscriptions.calls["trade"], ShouldBeEmpty)
			So(priceAPI.requested, ShouldBeEmpty)

			price.TickerAck([]byte(`{
				"channel":"ticker",
				"type":"snapshot",
				"data":[
					{"symbol":"SOL/USD","bid":"99.99","bid_qty":1,"ask":"100.01","ask_qty":1,"last":"100","volume":1,"vwap":100}
				]
			}`))

			Convey("Then exact-tier fees are hydrated before heavy feeds and readiness", func() {
				err = instrument.Subscribe()

				So(err, ShouldBeNil)
				So(priceAPI.requested, ShouldResemble, []string{"ETH/USD"})
				So(subscriptions.calls["trade"], ShouldResemble, [][]string{{"ETH/USD"}})
				So(subscriptions.calls["book"], ShouldResemble, [][]string{{"ETH/USD"}})
				So(subscriptions.calls["level3"], ShouldBeEmpty)
				So(price.Status(), ShouldEqual, types.READY)
				So(instrument.Status(), ShouldEqual, types.READY)
			})
		})
	})
}

func TestInstrumentRefreshLevel3(t *testing.T) {
	Convey("Given one symbol whose local level3 state diverged", t, func() {
		api := &instrumentAPIStub{}
		instrument := NewInstrument(api, nil, nil)

		Convey("When its level3 subscription is refreshed", func() {
			err := instrument.RefreshLevel3("BTC/USD")

			Convey("Then the old stream is removed before one replacement is subscribed", func() {
				So(err, ShouldBeNil)
				So(api.calls["level3-unsubscribe"], ShouldResemble, [][]string{{"BTC/USD"}})
				So(api.calls["level3"], ShouldResemble, [][]string{{"BTC/USD"}})
				So(api.order, ShouldResemble, []string{"level3-unsubscribe", "level3"})
			})
		})

		Convey("When the requested symbol is empty", func() {
			err := instrument.RefreshLevel3(" ")

			Convey("Then no subscription request is sent", func() {
				So(err, ShouldNotBeNil)
				So(api.order, ShouldBeEmpty)
			})
		})
	})
}

type instrumentAPIStub struct {
	calls map[string][][]string
	order []string
}

func (stub *instrumentAPIStub) SubscribeTicker(symbols []string) error {
	stub.record("ticker", symbols)
	return nil
}

func (stub *instrumentAPIStub) SubscribeTrade(symbols []string) error {
	stub.record("trade", symbols)
	return nil
}

func (stub *instrumentAPIStub) SubscribeBook(symbols []string) error {
	stub.record("book", symbols)
	return nil
}

func (stub *instrumentAPIStub) SubscribeOHLC(symbols []string) error {
	stub.record("ohlc", symbols)
	return nil
}

func (stub *instrumentAPIStub) SubscribeLevel3(symbols []string) error {
	stub.record("level3", symbols)
	return nil
}

func (stub *instrumentAPIStub) UnsubscribeLevel3(symbols []string) error {
	stub.record("level3-unsubscribe", symbols)
	return nil
}

func (stub *instrumentAPIStub) record(channel string, symbols []string) {
	if stub.calls == nil {
		stub.calls = map[string][][]string{}
	}

	stub.calls[channel] = append(
		stub.calls[channel],
		append([]string(nil), symbols...),
	)
	stub.order = append(stub.order, channel)
}

type instrumentPriceAPI struct {
	requested []string
}

func (stub *instrumentPriceAPI) On(_ string, _ func([]byte)) {}

func (stub *instrumentPriceAPI) TradeVolume(symbols []string) (*kraken.TradeVolume, error) {
	stub.requested = append([]string(nil), symbols...)
	fees := make(map[string]kraken.TradeVolumeFees, len(symbols))

	for _, symbol := range symbols {
		fees[symbol] = kraken.TradeVolumeFees{Fee: "0.2600"}
	}

	return &kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: fees},
	}, nil
}
