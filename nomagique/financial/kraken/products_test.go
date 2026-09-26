package kraken

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

/* productsFixture includes perpetual, dated, delisted and non-eligible contracts. */
const productsFixture = `{"result":"success","instruments":[
 {"symbol":"PF_XBTUSD","pair":"BTC:USD","tradeable":true},
 {"symbol":"PI_XBTUSD","pair":"BTC:USD","tradeable":true},
 {"symbol":"PF_ETHUSD","pair":"ETH:USD","tradeable":true},
 {"symbol":"PF_DOGEUSD","pair":"DOGE:USD","tradeable":true},
 {"symbol":"PF_DEADUSD","pair":"DEAD:USD","tradeable":false},
 {"symbol":"FI_XBTUSD_261225","pair":"BTC:USD","tradeable":true},
 {"symbol":"PF_USDTUSD","pair":"USDT:USD","tradeable":true}]}`

/* productsClient constructs the shared venue catalogue through its actual protocol. */
func productsClient(t testing.TB) Products {
	t.Helper()
	client := Products_ServerToClient(NewProducts())
	t.Cleanup(client.Release)
	if err := client.Write(context.Background(), func(params Products_write_Params) error { return productsInstruments(params, []byte(productsFixture)) }); err != nil {
		t.Fatal(err)
	}
	if err := client.WaitStreaming(); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestProductsWrite(t *testing.T) {
	Convey("The catalogue admits only listed tradeable perpetuals belonging to eligible spot pairs", t, func() {
		client := productsClient(t)
		send := func(symbols ...string) ([][]string, [][]string) {
			So(client.Write(context.Background(), func(params Products_write_Params) error {
				values, err := params.NewSymbols(int32(len(symbols)))
				if err != nil {
					return err
				}
				for index, symbol := range symbols {
					if err := values.Set(index, symbol); err != nil {
						return err
					}
				}
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			decode := func(frames capnp.DataList) [][]string {
				var subscriptions [][]string
				for index := range frames.Len() {
					raw, err := frames.At(index)
					So(err, ShouldBeNil)
					var frame struct {
						Event, Feed string
						Products    []string `json:"product_ids"`
					}
					So(json.Unmarshal(raw, &frame), ShouldBeNil)
					So(frame.Event, ShouldEqual, "subscribe")
					So(frame.Feed, ShouldEqual, []string{"ticker", "trade"}[index])
					subscriptions = append(subscriptions, frame.Products)
				}
				return subscriptions
			}
			delta, err := result.Subscribe()
			So(err, ShouldBeNil)
			handshake, err := result.OnConnect()
			So(err, ShouldBeNil)
			return decode(delta), decode(handshake)
		}
		delta, handshake := send("BTC/USD", "ETH/USD", "DEAD/USD", "NOFUTURE/USD")
		So(delta, ShouldResemble, [][]string{{"PF_ETHUSD", "PF_XBTUSD"}, {"PF_ETHUSD", "PF_XBTUSD"}})
		So(handshake, ShouldResemble, [][]string{{"PF_ETHUSD", "PF_XBTUSD"}, {"PF_ETHUSD", "PF_XBTUSD"}})
		Convey("Repeated snapshots emit no duplicate subscription", func() {
			delta, handshake := send("BTC/USD", "ETH/USD")
			So(delta, ShouldBeEmpty)
			So(handshake, ShouldBeEmpty)
		})
		Convey("A new spot listing emits only its delta and refreshes the complete reconnect handshake", func() {
			delta, handshake := send("DOGE/USD")
			So(delta, ShouldResemble, [][]string{{"PF_DOGEUSD"}, {"PF_DOGEUSD"}})
			So(handshake, ShouldResemble, [][]string{{"PF_DOGEUSD", "PF_ETHUSD", "PF_XBTUSD"}, {"PF_DOGEUSD", "PF_ETHUSD", "PF_XBTUSD"}})
		})
	})
	Convey("Malformed or failed instrument responses fail explicitly", t, func() {
		for _, payload := range []string{`{`, `{"result":"error"}`, `{"result":"success","instruments":[{"symbol":"PF_XBTUSD"}]}`} {
			client := Products_ServerToClient(NewProducts())
			So(client.Write(context.Background(), func(params Products_write_Params) error { return productsInstruments(params, []byte(payload)) }), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
			client.Release()
		}
	})
}

func TestProductsLookup(t *testing.T) {
	Convey("The venue Pair maps incoming contracts without invented aliases", t, func() {
		client := productsClient(t)
		for product, want := range map[string]string{"pf_xbtusd": "BTC/USD", "PF_DOGEUSD": "DOGE/USD", "FI_XBTUSD_261225": "BTC/USD"} {
			future, release := client.Lookup(context.Background(), func(params Products_lookup_Params) error { return params.SetProduct(product) })
			result, err := future.Struct()
			So(err, ShouldBeNil)
			symbol, err := result.Symbol()
			So(err, ShouldBeNil)
			So(symbol, ShouldEqual, want)
			release()
		}
		future, release := client.Lookup(context.Background(), func(params Products_lookup_Params) error { return params.SetProduct("PF_INVENTEDUSD") })
		defer release()
		_, err := future.Struct()
		So(err, ShouldNotBeNil)
	})
}

func BenchmarkProductsLookup(b *testing.B) {
	client := productsClient(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		future, release := client.Lookup(context.Background(), func(params Products_lookup_Params) error { return params.SetProduct("PF_XBTUSD") })
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductsWrite(b *testing.B) {
	// Match the 1,001-symbol admission fixture used by the compiled multi-shard feed test.
	const size = 1001
	type instrument struct {
		Symbol, Pair string
		Tradeable    bool
	}
	instruments := make([]instrument, size)
	symbols := make([]string, size)
	for index := range size {
		symbols[index] = fmt.Sprintf("COIN%d/USD", index)
		instruments[index] = instrument{fmt.Sprintf("PF_COIN%dUSD", index), fmt.Sprintf("COIN%d:USD", index), true}
	}
	payload, err := json.Marshal(struct {
		Result      string
		Instruments []instrument
	}{"success", instruments})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		client := Products_ServerToClient(NewProducts())
		if err := client.Write(context.Background(), func(params Products_write_Params) error {
			if err := productsInstruments(params, payload); err != nil {
				return err
			}
			values, err := params.NewSymbols(size)
			if err != nil {
				return err
			}
			for index, symbol := range symbols {
				if err := values.Set(index, symbol); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			client.Release()
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			client.Release()
			b.Fatal(err)
		}
		future, release := client.Done(context.Background(), nil)
		_, err := future.Struct()
		release()
		client.Release()
		if err != nil {
			b.Fatal(err)
		}
	}
}

/* productsInstruments supplies one catalogue response through the gathering port. */
func productsInstruments(params Products_write_Params, payload []byte) error {
	responses, err := params.NewInstruments(1)
	if err != nil {
		return err
	}
	return responses.Set(0, payload)
}
