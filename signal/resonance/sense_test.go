package resonance

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
)

func TestDefaultArchitecture(testingTB *testing.T) {
	Convey("Given the sensory channel contract", testingTB, func() {
		arch := DefaultArchitecture()

		So(len(arch), ShouldEqual, 3)
		So(arch[0], ShouldEqual, SensoryChannelCount)
		So(arch[1], ShouldEqual, SensoryChannelCount*2)
		So(arch[2], ShouldEqual, 3)
		So(validateArchitecture(arch), ShouldBeNil)
	})
}

func TestBuildSensoryVector(testingTB *testing.T) {
	Convey("Given ticker, book, and trade feeds", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ticker := feed.NewTicker(context.Background())
		book := feed.NewBook(context.Background())
		trade := feed.NewTrade(context.Background())
		registry := newSenseRegistry()
		scope := "BTC/USD"

		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		ticker.Update(krakenmarket.TickerUpdates{{
			Symbol:    scope,
			Last:      50000,
			Volume:    1200,
			ChangePct: 0.015,
			Timestamp: observedAt,
		}})

		book.Update(krakenmarket.BookUpdates{{
			Symbol: scope,
			Bids:   []krakenmarket.BookLevel{{Price: 49990, Qty: 2}},
			Asks:   []krakenmarket.BookLevel{{Price: 50010, Qty: 1}},
		}})

		trade.Update(krakenmarket.TradeUpdates{
			&krakenmarket.TradeUpdate{
				Symbol:    scope,
				Price:     50000,
				Qty:       0.5,
				Side:      "buy",
				Timestamp: observedAt,
			},
			&krakenmarket.TradeUpdate{
				Symbol:    scope,
				Price:     50001,
				Qty:       0.4,
				Side:      "sell",
				Timestamp: observedAt.Add(time.Second),
			},
		})

		vector, facts, ok := buildSensoryVector(scope, ticker, book, trade, registry)

		Convey("It should emit twelve normalized sensory channels", func() {
			So(ok, ShouldBeTrue)
			So(len(vector), ShouldEqual, SensoryChannelCount)
			So(facts.lastPrice, ShouldEqual, 50000)
			So(vector[0], ShouldBeGreaterThan, 0)
			So(vector[1], ShouldBeGreaterThan, 0)
		})
	})
}

func TestMeasureTargets(testingTB *testing.T) {
	cases := []struct {
		category logic.CategoryType
		expected []string
	}{
		{
			category: logic.CategoryType(CategoryFlow),
			expected: []string{"fluid", "depthflow", "exhaust", "liquidity"},
		},
		{
			category: logic.CategoryType(CategoryStress),
			expected: []string{"toxicity", "hawkes", "pumpdump", "cvd"},
		},
		{
			category: logic.CategoryType(CategoryCoupling),
			expected: []string{
				"correlation", "leadlag", "causal", "sentiment", "manifold", "prediction",
			},
		},
	}

	for _, testCase := range cases {
		Convey("Given resonance attention mode "+string(testCase.category), testingTB, func() {
			So(MeasureTargets(testCase.category), ShouldResemble, testCase.expected)
		})
	}
}
