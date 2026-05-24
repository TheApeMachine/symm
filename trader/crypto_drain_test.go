package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

type countingTicker struct {
	pending int
	ticks   int
}

func (ticker *countingTicker) Tick() bool {
	if ticker.pending <= 0 {
		return false
	}

	ticker.pending--
	ticker.ticks++

	return true
}

type batchDrainTicker struct {
	pending      int
	tickCalls    int
	limitHistory []int
}

func (ticker *batchDrainTicker) Tick() bool {
	ticker.tickCalls++

	return false
}

func (ticker *batchDrainTicker) Drain(limit int) int {
	ticker.limitHistory = append(ticker.limitHistory, limit)

	drained := min(limit, ticker.pending)
	ticker.pending -= drained

	return drained
}

type steadyDrainTicker struct {
	drained int
}

func (ticker *steadyDrainTicker) Tick() bool {
	return false
}

func (ticker *steadyDrainTicker) Drain(limit int) int {
	ticker.drained += limit

	return limit
}

func TestDrainTickables(t *testing.T) {
	originalPerSignal := config.System.MaxPendingPerSignal
	originalGlobal := config.System.MaxPendingGlobal

	t.Cleanup(func() {
		config.System.MaxPendingPerSignal = originalPerSignal
		config.System.MaxPendingGlobal = originalGlobal
	})

	convey.Convey("Given tickers with more pending work than the configured budget", t, func() {
		config.System.MaxPendingPerSignal = 3
		config.System.MaxPendingGlobal = 5

		firstTicker := &countingTicker{pending: 10}
		secondTicker := &countingTicker{pending: 10}
		crypto := &Crypto{tickers: []engine.Ticker{firstTicker, secondTicker}}

		crypto.drainTickables()

		convey.Convey("It should stop at the global drain budget", func() {
			convey.So(firstTicker.ticks, convey.ShouldEqual, 3)
			convey.So(secondTicker.ticks, convey.ShouldEqual, 2)
			convey.So(firstTicker.pending, convey.ShouldEqual, 7)
			convey.So(secondTicker.pending, convey.ShouldEqual, 8)
		})
	})

	convey.Convey("Given a ticker with a batch drain implementation", t, func() {
		config.System.MaxPendingPerSignal = 4
		config.System.MaxPendingGlobal = 0

		batchTicker := &batchDrainTicker{pending: 10}
		fallbackTicker := &countingTicker{pending: 10}
		crypto := &Crypto{tickers: []engine.Ticker{batchTicker, fallbackTicker}}

		crypto.drainTickables()

		convey.Convey("It should use the batch drain and derive the global budget from registered tickers", func() {
			convey.So(batchTicker.tickCalls, convey.ShouldEqual, 0)
			convey.So(batchTicker.limitHistory, convey.ShouldResemble, []int{4})
			convey.So(batchTicker.pending, convey.ShouldEqual, 6)
			convey.So(fallbackTicker.ticks, convey.ShouldEqual, 4)
			convey.So(fallbackTicker.pending, convey.ShouldEqual, 6)
		})
	})
}

func BenchmarkDrainTickables(benchmark *testing.B) {
	originalPerSignal := config.System.MaxPendingPerSignal
	originalGlobal := config.System.MaxPendingGlobal

	benchmark.Cleanup(func() {
		config.System.MaxPendingPerSignal = originalPerSignal
		config.System.MaxPendingGlobal = originalGlobal
	})

	config.System.MaxPendingPerSignal = 4096
	config.System.MaxPendingGlobal = 0

	crypto := &Crypto{tickers: []engine.Ticker{
		&steadyDrainTicker{},
		&steadyDrainTicker{},
		&steadyDrainTicker{},
		&steadyDrainTicker{},
	}}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		crypto.drainTickables()
	}
}
