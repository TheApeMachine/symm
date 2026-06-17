package depthflow

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	feed "github.com/theapemachine/symm/signal"
)

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
}

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func measurementQuery(scope string) datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return *acquired
}

func observeRow(
	signal *Signal,
	symbol string,
	price, value, volume, pressure float64,
	eventAt time.Time,
) {
	row, err := krakenmarket.NewSymbolRow(symbol, price, value, volume, pressure, eventAt)

	if err != nil {
		panic(err)
	}

	if err := signal.CrossSection.Observe(row); err != nil {
		panic(err)
	}
}

type bookFrame struct {
	bids []krakenmarket.BookLevel
	asks []krakenmarket.BookLevel
}

func seedBooks(
	signal *Signal,
	symbol string,
	base time.Time,
	frames []bookFrame,
) {
	for index, frame := range frames {
		update := &krakenmarket.BookUpdate{
			Symbol:    symbol,
			Type:      "snapshot",
			Bids:      frame.bids,
			Asks:      frame.asks,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}

		signal.book.Update(feed.BookFeedArtifact(krakenmarket.BookUpdates{update}))
	}
}

func bidHeavyFrames() []bookFrame {
	return []bookFrame{
		{
			bids: []krakenmarket.BookLevel{{Price: 99, Qty: 10}, {Price: 98, Qty: 20}},
			asks: []krakenmarket.BookLevel{{Price: 101, Qty: 1}, {Price: 102, Qty: 1}},
		},
		{
			bids: []krakenmarket.BookLevel{{Price: 99, Qty: 12}, {Price: 98, Qty: 22}},
			asks: []krakenmarket.BookLevel{{Price: 101, Qty: 1}, {Price: 102, Qty: 1}},
		},
		{
			bids: []krakenmarket.BookLevel{{Price: 99, Qty: 14}, {Price: 98, Qty: 24}},
			asks: []krakenmarket.BookLevel{{Price: 101, Qty: 1}, {Price: 102, Qty: 1}},
		},
	}
}

func spoofFrames() []bookFrame {
	heavyBid := krakenmarket.BookLevel{Price: 48, Qty: 30}
	lightBid := krakenmarket.BookLevel{Price: 49, Qty: 1}
	heavyTouchBid := krakenmarket.BookLevel{Price: 49, Qty: 2}
	asks := []krakenmarket.BookLevel{{Price: 51, Qty: 8}, {Price: 52, Qty: 8}}

	return []bookFrame{
		{bids: []krakenmarket.BookLevel{lightBid, heavyBid}, asks: asks},
		{bids: []krakenmarket.BookLevel{heavyTouchBid, heavyBid}, asks: asks},
		{bids: []krakenmarket.BookLevel{heavyTouchBid, heavyBid}, asks: asks},
		{bids: []krakenmarket.BookLevel{lightBid, heavyBid}, asks: asks},
		{bids: []krakenmarket.BookLevel{heavyTouchBid, heavyBid}, asks: asks},
		{bids: []krakenmarket.BookLevel{heavyTouchBid, heavyBid}, asks: asks},
		{bids: []krakenmarket.BookLevel{heavyTouchBid, heavyBid}, asks: asks},
	}
}

func TestSignalMeasure(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a bid-heavy book", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "BTC/EUR", 100, 1, 10000, 0.8, eventAt)
		seedBooks(signal, "BTC/EUR", eventAt, bidHeavyFrames())

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify loaded imbalance", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given deep bid wall with bearish touch", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		observeRow(signal, "ETH/EUR", 50, 1, 10000, -0.5, eventAt)
		seedBooks(signal, "ETH/EUR", eventAt, spoofFrames())

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify spoof trap", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given trade pressure update", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		signal.trade.Update(feed.TradeFeedArtifact(krakenmarket.TradeUpdates{
			{
				Symbol:    "SOL/EUR",
				Side:      "buy",
				Price:     25,
				Qty:       2,
				Timestamp: eventAt,
			},
			{
				Symbol:    "SOL/EUR",
				Side:      "buy",
				Price:     25.1,
				Qty:       3,
				Timestamp: eventAt.Add(time.Millisecond),
			},
		}))

		signal.Measure(measurementQuery("SOL/EUR"))

		Convey("It should observe trade pressure while awaiting book", func() {
			pressure := signal.CrossSection.Pressure("SOL/EUR")
			So(pressure, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureBeforeUniverseEntry(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a book update before the symbol enters the cross-section", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		seedBooks(signal, "ZBCN/USD", eventAt, bidHeavyFrames())

		result := signal.Measure(measurementQuery("ZBCN/USD"))

		Convey("It should measure without halting on missing trade pressure", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ZBCN/USD")
		})
	})
}

func TestSignalMeasureWithholdsUntilBookHistoryReady(testingTB *testing.T) {
	Convey("Given one book update in the ring", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		signal.book.Update(feed.BookFeedArtifact(krakenmarket.BookUpdates{{
			Symbol: "BTC/EUR",
			Type:   "snapshot",
			Bids:   []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
			Asks:   []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
		}}))

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should withhold until enough ring history exists", func() {
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	query := measurementQuery("BTC/EUR")
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), pool)

		observeRow(signal, "BTC/EUR", 100, 1, 10000, 0.8, eventAt)
		seedBooks(signal, "BTC/EUR", eventAt, bidHeavyFrames())

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}
	}
}
