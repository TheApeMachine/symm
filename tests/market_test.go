package tests

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
collect drains every ready message from an Actor subscription.
*/
func collect(sub *types.Subscription) []any {
	frames := make([]any, 0)

	for {
		select {
		case frame := <-sub.Channel:
			frames = append(frames, frame)
		default:
			return frames
		}
	}
}

/*
await waits until at least n Actor frames arrive or the deadline expires.
*/
func await(sub *types.Subscription, n int) []any {
	frames := make([]any, 0, n)
	deadline := time.Now().Add(2 * time.Second)

	for len(frames) < n && time.Now().Before(deadline) {
		select {
		case frame := <-sub.Channel:
			frames = append(frames, frame)
		case <-time.After(5 * time.Millisecond):
		}
	}

	return frames
}

/*
TestMarket_Apply proves explicit steps become coherent Kraken frames on the
Actor roots without requiring a trade per update.
*/
func TestMarket_Apply(t *testing.T) {
	Convey("Given a bootstrapped two-symbol market at an explicit time", t, func() {
		start := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
		market := NewMarket(t.Context(), 2, MarketOptions{Start: start})
		So(market.Bootstrap(), ShouldBeNil)
		Reset(market.Close)

		tradeSub := market.Public.Subscribe("trade")
		bookSub := market.Public.Subscribe("book")
		tickerSub := market.Public.Subscribe("ticker")
		level3Sub := market.Level3.Subscribe("level3")

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

		collect(tradeSub)
		collect(bookSub)
		collect(tickerSub)
		collect(level3Sub)
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

			books := await(bookSub, 1)
			trades := collect(tradeSub)
			So(books, ShouldHaveLength, 1)
			So(trades, ShouldBeEmpty)

			book := books[0].(*kraken.Book)
			So(book.Data[0].PriceIncrement, ShouldNotBeNil)
			So(book.Data[0].PriceIncrement.Float64(), ShouldBeGreaterThan, 0)

			So(market.Public.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD"]}}`,
			)), ShouldBeNil)
			books = await(bookSub, 1)
			So(books, ShouldHaveLength, 1)
			book = books[0].(*kraken.Book)
			So(book.Type, ShouldEqual, "snapshot")
			So(book.Data, ShouldHaveLength, 1)
			So(book.Data[0].Symbol, ShouldEqual, "SIM1/USD")

			So(market.Public.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD"]}}`,
			)), ShouldBeNil)
			trades = await(tradeSub, 1)
			So(trades, ShouldHaveLength, 1)
			trade := trades[0].(*kraken.Trade)
			So(trade.Type, ShouldEqual, "snapshot")
			So(trade.Data, ShouldHaveLength, 1)
			So(trade.Data[0].Symbol, ShouldEqual, "SIM1/USD")
		})

		Convey("An empty observation invokes the system without fabricating source frames", func() {
			So(market.Apply(MarketStep{Advance: time.Second}, func() error {
				steps++
				return nil
			}), ShouldBeNil)
			So(steps, ShouldEqual, 1)
			time.Sleep(20 * time.Millisecond)
			So(collect(tradeSub), ShouldBeEmpty)
			So(collect(bookSub), ShouldBeEmpty)
			So(collect(tickerSub), ShouldBeEmpty)
			So(collect(level3Sub), ShouldBeEmpty)
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
			trades := await(tradeSub, 1)
			So(trades, ShouldHaveLength, 1)
			trade := trades[0].(*kraken.Trade)
			So(trade.Data, ShouldHaveLength, 2)
			So(trade.Data[0].Price.Float64(), ShouldNotEqual, trade.Data[1].Price.Float64())
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

		Convey("A production feed rejection makes later application fail-stop", func() {
			So(market.Apply(MarketStep{
				Advance: time.Second,
				Actions: []MarketAction{{
					Kind: MarketTrade, Symbol: "SIM1/USD", Side: "buy", Qty: 1,
				}},
			}, func() error {
				market.Public.Report(errors.New("ticker rejected"))
				return nil
			}), ShouldBeNil)
			failedAt := market.Now()
			So(market.Apply(MarketStep{
				Advance: time.Second,
			}, func() error { return nil }), ShouldNotBeNil)
			So(market.Now(), ShouldEqual, failedAt)
		})
	})

	Convey("Given two identical multi-symbol markets", t, func() {
		run := func() []string {
			market := NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			defer market.Close()

			level3Sub := market.Level3.Subscribe("level3")
			bookSub := market.Public.Subscribe("book")
			tradeSub := market.Public.Subscribe("trade")
			tickerSub := market.Public.Subscribe("ticker")

			So(market.Level3.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"level3","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"],"depth":10}}`,
			)), ShouldBeNil)
			So(market.Public.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
			)), ShouldBeNil)
			So(market.Public.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
			)), ShouldBeNil)
			So(market.Public.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"ticker","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
			)), ShouldBeNil)

			collect(level3Sub)
			collect(bookSub)
			collect(tradeSub)
			collect(tickerSub)

			So(market.Apply(MarketStep{
				Advance: time.Second,
				Actions: []MarketAction{
					{Kind: MarketTrade, Symbol: "SIM1/USD", Side: "buy", Qty: 1},
					{Kind: MarketTrade, Symbol: "SIM2/USD", Side: "sell", Qty: 1},
					{Kind: MarketTrade, Symbol: "SIM3/USD", Side: "buy", Qty: 1},
				},
			}, func() error { return nil }), ShouldBeNil)

			frames := make([]string, 0)

			for _, frame := range await(level3Sub, 1) {
				frames = append(frames, "level3:"+string(frame.([]byte)))
			}

			for _, frame := range await(bookSub, 1) {
				raw, err := json.Marshal(frame.(*kraken.Book))
				So(err, ShouldBeNil)
				frames = append(frames, "book:"+string(raw))
			}

			for _, frame := range await(tradeSub, 1) {
				raw, err := json.Marshal(frame.(*kraken.Trade))
				So(err, ShouldBeNil)
				frames = append(frames, "trade:"+string(raw))
			}

			for _, frame := range await(tickerSub, 1) {
				raw, err := json.Marshal(frame.(*kraken.Ticker))
				So(err, ShouldBeNil)
				frames = append(frames, "ticker:"+string(raw))
			}

			return frames
		}

		Convey("Their frames and global trade IDs should be byte-identical", func() {
			So(run(), ShouldResemble, run())
		})
	})
}

/*
TestMarket_Bootstrap proves no event can enter the simulated venue before its
Kraken snapshots establish complete trade, ticker, book, and Level3 state.
*/
func TestMarket_Bootstrap(t *testing.T) {
	Convey("Given a market whose source snapshots have not been installed", t, func() {
		market := NewMarket(t.Context(), 1)
		invocations := 0
		callback := func() error {
			invocations++
			return nil
		}

		So(market.Apply(MarketStep{Advance: time.Second}, callback), ShouldNotBeNil)
		So(market.Warmup(callback), ShouldNotBeNil)
		So(market.Transition(MarketStateBaseline, callback), ShouldNotBeNil)
		So(invocations, ShouldEqual, 0)

		market.Close()
		So(market.Bootstrap(), ShouldNotBeNil)
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
				{Kind: MarketRefill, Symbol: "SIM1/USD", Side: "sell", Qty: 1},
				{Kind: MarketTrade, Symbol: "SIM2/USD", Side: "sell", Qty: 1},
				{Kind: MarketRefill, Symbol: "SIM2/USD", Side: "buy", Qty: 1},
				{Kind: MarketTrade, Symbol: "SIM3/USD", Side: "buy", Qty: 1},
				{Kind: MarketRefill, Symbol: "SIM3/USD", Side: "sell", Qty: 1},
			},
		}, func() error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}
