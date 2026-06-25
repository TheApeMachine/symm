package resonance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestDefaultArchitecture(testingTB *testing.T) {
	Convey("Given the sensory channel contract", testingTB, func() {
		arch := DefaultArchitecture()

		So(len(arch), ShouldEqual, 3)
		So(arch[0], ShouldEqual, SensoryChannelCount)
		So(arch[1], ShouldBeGreaterThanOrEqualTo, SensoryChannelCount*2)
		So(arch[2], ShouldEqual, resonanceLatentWidth)
		So(validateArchitecture(arch), ShouldBeNil)
	})
}

func TestBuildSensoryVector(testingTB *testing.T) {
	Convey("Given ticker and book fixtures in market buffers", testingTB, func() {
		ctx := context.Background()
		ticker := newMarketTicker(ctx)
		book := newMarketBook(ctx)
		trade := newMarketTrade(ctx)
		registry := newSenseRegistry()
		scope := "BTC/USD"
		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		tickerRaw, err := json.Marshal(tickerFixture{
			Symbol:    scope,
			Last:      50000,
			Volume:    1200,
			ChangePct: 0.015,
			Timestamp: observedAt,
		})

		So(err, ShouldBeNil)

		bookRaw, err := json.Marshal(bookFixture{
			Symbol: scope,
			Bids:   []bookLevelFixture{{Price: 49990, Qty: 1}},
			Asks:   []bookLevelFixture{{Price: 50010, Qty: 1}},
		})

		So(err, ShouldBeNil)

		ticker.ingest(scope, tickerRaw, observedAt)
		book.ingest(scope, bookRaw, observedAt)

		vector, facts, ok := buildSensoryVector(scope, ticker, book, trade, registry)

		Convey("It should build a twelve-channel sensory vector", func() {
			So(ok, ShouldBeTrue)
			So(len(vector), ShouldEqual, SensoryChannelCount)
			So(facts.lastPrice, ShouldEqual, 50000)
		})
	})
}

func TestAttentionCategoryIndex(testingTB *testing.T) {
	Convey("Given latent reconstruction modes", testingTB, func() {
		So(AttentionCategoryIndex(0, []float64{0.1, 0.2, 0.3}), ShouldEqual, 3)
		So(AttentionCategoryIndex(1.5, []float64{0.1, 0.9, 0.2}), ShouldEqual, 2)
		So(AttentionCategoryIndex(1.5, []float64{0.9, 0.1, 0.2}), ShouldEqual, 1)
		So(AttentionConfidence(1.5, 0.2, []float64{0.9, 0.1, 0.2}), ShouldBeGreaterThan, 0)
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
				"correlation", "leadlag", "causal", "sentiment", "manifold",
			},
		},
	}

	for _, testCase := range cases {
		Convey("Given resonance attention mode "+string(testCase.category), testingTB, func() {
			So(MeasureTargets(testCase.category), ShouldResemble, testCase.expected)
		})
	}
}
