package tests

import (
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
		market.Public.On("trade", func(payload []byte) { trades = append(trades, payload) })
		market.Public.On("book", func(payload []byte) { books = append(books, payload) })
		steps := 0

		Convey("A refill emits a book-only state and advances the fake clock once", func() {
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
			So(trades, ShouldHaveLength, 1)
			So(string(trades[0]), ShouldContainSubstring, `"data":[]`)
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
