package tests

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestMarket_Apply proves explicit steps become coherent Kraken frames through
the deterministic connection scheduler without requiring a trade per update.
*/
func TestMarket_Apply(t *testing.T) {
	Convey("Given a bootstrapped two-symbol market at an explicit time", t, func() {
		start := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
		market := NewMarket(t.Context(), 2, MarketOptions{Start: start})
		So(market.Bootstrap(), ShouldBeNil)
		Reset(market.Close)
		trades := [][]byte{}
		books := [][]byte{}
		tickers := [][]byte{}
		level3 := [][]byte{}
		market.Public.On("trade", func(payload []byte) { trades = append(trades, payload) })
		market.Public.On("book", func(payload []byte) { books = append(books, payload) })
		market.Public.On("ticker", func(payload []byte) { tickers = append(tickers, payload) })
		market.Level3.On("level3", func(payload []byte) { level3 = append(level3, payload) })
		So(market.Public.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD"]}}`,
		)), ShouldBeNil)
		So(market.Public.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD"]}}`,
		)), ShouldBeNil)
		So(market.Public.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD"]}}`,
		)), ShouldBeNil)
		So(market.Level3.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"level3","symbol":["SIM1/USD"],"depth":10}}`,
		)), ShouldBeNil)
		trades = nil
		books = nil
		tickers = nil
		level3 = nil
		steps := 0

		Convey("A refill emits no fabricated trade frame and advances the clock once", func() {
			err := market.Apply(MarketStep{
				Advance: time.Second,
				Actions: []MarketAction{{
					Kind: MarketRefill, Symbol: "SIM1/USD", Side: "buy", Qty: 5,
				}},
			}, func() error {
				steps++
				return nil
			})
			So(err, ShouldBeNil)
			So(steps, ShouldEqual, 1)
			So(market.Now(), ShouldEqual, start.Add(time.Second))
			So(books, ShouldHaveLength, 1)
			So(trades, ShouldBeEmpty)

			So(market.Public.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD"]}}`,
			)), ShouldBeNil)
			So(books, ShouldHaveLength, 2)
			So(string(books[1]), ShouldContainSubstring, `"type":"snapshot"`)
			So(string(books[1]), ShouldContainSubstring, `"symbol":"SIM1/USD"`)
			So(string(books[1]), ShouldNotContainSubstring, `"symbol":"SIM2/USD"`)
			So(market.Public.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD"]}}`,
			)), ShouldBeNil)
			So(trades, ShouldHaveLength, 1)
			So(string(trades[0]), ShouldContainSubstring, `"type":"snapshot"`)
			So(string(trades[0]), ShouldContainSubstring, `"symbol":"SIM1/USD"`)
		})

		Convey("An empty observation invokes the system without fabricating source frames", func() {
			So(market.Apply(MarketStep{Advance: time.Second}, func() error {
				steps++
				return nil
			}), ShouldBeNil)
			So(steps, ShouldEqual, 1)
			So(trades, ShouldBeEmpty)
			So(books, ShouldBeEmpty)
			So(tickers, ShouldBeEmpty)
			So(level3, ShouldBeEmpty)
		})

		Convey("A buy consumes its complete touch and continues through the next level", func() {
			quote, exists := market.signal.Quote("SIM1/USD")
			So(exists, ShouldBeTrue)
			So(market.Apply(MarketStep{
				Advance: time.Second,
				Actions: []MarketAction{{
					Kind:   MarketTrade,
					Symbol: "SIM1/USD",
					Side:   "buy",
					Qty:    quote.AskQty + 5,
				}},
			}, func() error { return nil }), ShouldBeNil)
			So(trades, ShouldHaveLength, 1)
			var frame wireFrame[wireTrade]
			So(json.Unmarshal(trades[0], &frame), ShouldBeNil)
			So(frame.Data, ShouldHaveLength, 2)
			So(frame.Data[0].Price, ShouldNotEqual, frame.Data[1].Price)
		})

		Convey("Unknown states and closed markets fail before invoking the step", func() {
			So(market.Transition(MarketState(255), func() error {
				steps++
				return nil
			}), ShouldNotBeNil)
			So(steps, ShouldEqual, 0)
			market.Close()
			So(market.Apply(MarketStep{Advance: time.Second}, func() error {
				steps++
				return nil
			}), ShouldNotBeNil)
			So(steps, ShouldEqual, 0)
		})

		Convey("A second bootstrap is rejected without changing event time", func() {
			So(market.Bootstrap(), ShouldNotBeNil)
			So(market.Now(), ShouldEqual, start)
		})

		Convey("A callback failure makes later market application unambiguous", func() {
			callbackErr := errors.New("consumer failed")
			So(market.Apply(MarketStep{
				Advance: time.Second,
				Actions: []MarketAction{{
					Kind: MarketRefill, Symbol: "SIM1/USD", Side: "buy", Qty: 1,
				}},
			}, func() error { return callbackErr }), ShouldNotBeNil)
			So(market.Apply(MarketStep{
				Advance: time.Second,
			}, func() error { return nil }), ShouldNotBeNil)
			So(market.Now(), ShouldEqual, start.Add(time.Second))
		})
	})

	Convey("Given two identical multi-symbol markets", t, func() {
		run := func() [][]byte {
			market := NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			defer market.Close()
			frames := [][]byte{}

			for _, subscription := range []struct {
				conn interface {
					On(string, func([]byte)) uint64
					Write(json.Marshaler) error
				}
				channel string
				request json.RawMessage
			}{
				{market.Level3, "level3", json.RawMessage(
					`{"method":"subscribe","params":{"channel":"level3","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"],"depth":10}}`,
				)},
				{market.Public, "book", json.RawMessage(
					`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
				)},
				{market.Public, "trade", json.RawMessage(
					`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
				)},
				{market.Public, "ticker", json.RawMessage(
					`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
				)},
			} {
				channel := subscription.channel
				subscription.conn.On(channel, func(payload []byte) {
					frames = append(frames, append([]byte(channel+":"), payload...))
				})
				So(subscription.conn.Write(subscription.request), ShouldBeNil)
			}

			frames = nil
			So(market.Apply(MarketStep{
				Advance: time.Second,
				Actions: []MarketAction{
					{Kind: MarketTrade, Symbol: "SIM1/USD", Side: "buy", Qty: 1},
					{Kind: MarketTrade, Symbol: "SIM2/USD", Side: "sell", Qty: 1},
					{Kind: MarketTrade, Symbol: "SIM3/USD", Side: "buy", Qty: 1},
				},
			}, func() error { return nil }), ShouldBeNil)
			return frames
		}

		Convey("Their frames and global trade IDs should be byte-identical", func() {
			So(run(), ShouldResemble, run())
		})
	})
}

/*
BenchmarkMarket_Apply measures one complete generated, validated, and delivered
market step through the mock Conn boundary.
*/
func BenchmarkMarket_Apply(b *testing.B) {
	market := NewMarket(b.Context(), 3)

	if err := market.Bootstrap(); err != nil {
		b.Fatal(err)
	}

	defer market.Close()
	b.ReportAllocs()

	for b.Loop() {
		if err := market.Apply(MarketStep{
			Advance: time.Second,
			Actions: []MarketAction{
				{Kind: MarketTrade, Symbol: "SIM1/USD", Side: "buy", Qty: 1},
				{Kind: MarketTrade, Symbol: "SIM2/USD", Side: "sell", Qty: 1},
				{Kind: MarketTrade, Symbol: "SIM3/USD", Side: "buy", Qty: 1},
			},
		}, func() error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
