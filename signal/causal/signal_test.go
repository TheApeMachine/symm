package causal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
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

func feedTrade(
	signal *Signal,
	symbol, side string,
	price, qty float64,
	at time.Time,
) {
	raw, err := json.Marshal(map[string]any{
		"symbol":    symbol,
		"side":      side,
		"price":     price,
		"qty":       qty,
		"timestamp": at,
	})

	if err != nil {
		panic(err)
	}

	signal.nodeStore.Observe(symbol, raw)
}

func TestSignalMeasureWithholdsUntilLadderSettles(testingTB *testing.T) {
	Convey("Given a causal signal with insufficient ladder history", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		feedTrade(
			signal,
			"BTC/USD",
			"buy",
			100,
			0.1,
			time.Now(),
		)
		result := signal.Measure(measurementQuery("BTC/USD"))

		Convey("It should withhold the measurement", func() {
			So(result, ShouldBeNil)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a causal signal fed with trades", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		baseTime := time.Now()
		price := 100.0

		for index := range 64 {
			wobble := float64((index*7)%13) * 0.5
			side := "buy"

			if index%3 == 0 {
				side = "sell"
			}

			feedTrade(
				signal,
				"BTC/USD",
				side,
				price+wobble,
				0.1+wobble*0.04,
				baseTime.Add(time.Duration(index)*time.Second),
			)
		}

		result := signal.Measure(measurementQuery("BTC/USD"))

		Convey("It should derive category through inline nomagique.Number", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/USD")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalTradeObserve(testingTB *testing.T) {
	Convey("Given a causal signal", testingTB, func() {
		signal := NewSignal(
			context.Background(),
			newTestPool(testingTB),
		)

		feedTrade(signal, "BTC/USD", "buy", 1.0, 0.1, time.Now())

		Convey("It should store the trade per symbol in the node store", func() {
			nodes := signal.nodeStore.Nodes("BTC/USD")
			So(nodes, ShouldNotBeNil)

			buf := make([]byte, 4096)
			n, _ := nodes.Read(buf)

			So(n, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		context.Background(),
		newTestPool(b),
	)

	baseTime := time.Now()
	price := 100.0

	for index := range 16 {
		feedTrade(
			signal,
			"BTC/USD",
			"buy",
			price+float64(index)*0.5,
			0.1,
			baseTime.Add(time.Duration(index)*time.Second),
		)
	}

	query := measurementQuery("BTC/USD")

	b.ReportAllocs()

	for b.Loop() {
		_ = signal.Measure(query)
	}
}
