package pumpdump

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/tests"
)

func categoryResult(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

func newTestPool(t testing.TB) *qpool.Q[any] {
	if t != nil {
		t.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && t != nil {
		t.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func tickerQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("kraken:public", datura.APPJSON)
	acquired.WithRole("ticker")
	acquired.WithScope(scope)

	return acquired
}

func krakenTickerFrame(
	volume, vwap, last, bid, ask, changePct float64,
	scope string,
) []byte {
	return fmt.Appendf(nil,
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"bid":%g,"bid_qty":740.0,"ask":%g,"ask_qty":740.0,"last":%g,"volume":%g,"vwap":%g,"change_pct":%g}]}`,
		scope, bid, ask, last, volume, vwap, changePct,
	)
}

// replaySequence drives deterministic, strictly-increasing replay timestamps.
// Based at the current day so stored rows share the query's date segment in the
// key, then incremented per insert to fix replay order.
var replaySequence = time.Now().UnixNano()

const tickerUpdatePrefix = "ticker/update"

func measureTickerFrame(
	signal *Signal,
	symbol string,
	volume, vwap, last, bid, ask, changePct float64,
) *datura.Artifact {
	stored := datura.Acquire("kraken:public", datura.APPJSON)
	stored.WithRole("ticker")
	stored.WithScope("update")
	stored.WithPayload(krakenTickerFrame(volume, vwap, last, bid, ask, changePct, symbol))

	return signal.Measure(stored)
}

func warmupTickerFrames(
	signal *Signal,
	symbol string,
	tickCount int,
	volumeStep, vwap, last, bid, ask, changePct float64,
) *datura.Artifact {
	var result *datura.Artifact

	for tick := range tickCount {
		volume := volumeStep * float64(tick+1)
		result = measureTickerFrame(signal, symbol, volume, vwap, last, bid, ask, changePct)
	}

	return result
}

func measureStoredReplay(signal *Signal, tree *dmt.Tree) *datura.Artifact {
	var result *datura.Artifact

	for stored := range tree.Seek([]byte(tickerUpdatePrefix)) {
		result = signal.Measure(stored)
	}

	return result
}

func verticalIgnitionTicker() (float64, float64, float64, float64, float64, float64) {
	return 11000, 10000, 41000, 40990, 41010, 3.1
}

func coiledCompressionTicker() (float64, float64, float64, float64, float64, float64) {
	// Warmup deltas are 120; a 1.5x delta is moderate lift without ignition spike.
	return 7260, 10000, 10050, 10050.0001, 10050.0002, 0.05
}

func organicTrendTicker() (float64, float64, float64, float64, float64, float64) {
	return 10200, 10000, 12500, 12490, 12510, 0.4
}

func fadedExhaustionTicker() (float64, float64, float64, float64, float64, float64) {
	// Warmup adds 200 per cumulative tick; a 1 increment is sharply fading lift.
	return 11801, 10000, 10100, 10080, 10120, 0.05
}

func TestScopePrefix(t *testing.T) {
	Convey("Given a query artifact with a slash-bearing scope", t, func() {
		query := tickerQuery("BTC/USD")

		defer query.Release()

		Convey("It should build the role/scope seek prefix", func() {
			So(string(query.Prefix("role", "scope")), ShouldEqual, "ticker/BTC/USD")
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a vertical ignition ticker update", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
		warmupTickerFrames(signal, "ETH/EUR", 59, 100, vwap, 10000, 9990, 10010, 0)
		result := measureTickerFrame(signal, "ETH/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should classify vertical ignition from the ticker replay", func() {
			t.Logf(
				"vertical category=%d ignition=%v trend=%v",
				categoryResult(result),
				datura.Peek[float64](result, "output", "ignition"),
				datura.Peek[float64](result, "output", "trend"),
			)
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 1)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given spread compression with low precursor", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()

		var result *datura.Artifact

		for tick := range 59 {
			volumeStep := 120.0 * float64(tick+1)
			result = measureTickerFrame(
				signal, "BTC/EUR", volumeStep, vwap, 10050, 10040, 10060, 0,
			)
			result.Release()
		}

		result = measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should classify coiled compression from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 2)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given steady momentum without vertical lift", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := organicTrendTicker()

		var result *datura.Artifact

		for tick := range 59 {
			volumeStep := 100.0 * float64(tick+1)
			warmupLast := 12400.0 + float64(tick)*2.0
			result = measureTickerFrame(
				signal, "TREND/EUR", volumeStep, vwap, warmupLast, 12490, 12510, 0.15,
			)
			result.Release()
		}

		result = measureTickerFrame(signal, "TREND/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should classify organic trend from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 3)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given fading volume lift with flat precursor", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := fadedExhaustionTicker()
		warmupTickerFrames(signal, "FADE/EUR", 59, 200, vwap, 10100, 10095, 10105, 0.05)
		result := measureTickerFrame(signal, "FADE/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should classify faded exhaustion from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 4)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a sparse tree at startup", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := warmupTickerFrames(signal, "NEW/EUR", 0, 100, 10000, 10000, 9990, 10010, 0)

		Convey("It should leave the query unclassified without ticker rows", func() {
			So(result, ShouldBeNil)
		})
	})
}

func TestMeasureReplayTraversal(t *testing.T) {
	Convey("Given a long ticker replay through the full pumpdump pipeline", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupTickerFrames(signal, "REPLAY/USD", 120, 100, 10000, 10000, 9990, 10010, 0)

		volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
		result := measureTickerFrame(signal, "REPLAY/USD", volume, vwap, last, bid, ask, changePct)

		Convey("It should complete replay without losing classifier output", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestIntegration(t *testing.T) {
	Convey("Given a pumpdump signal", t, func() {
		signal := NewSignal(
			t.Context(),
			newTestPool(t),
			dmt.NewTree(""),
		)

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		Convey("And a ticker datapoint", func() {
			datapoint := tests.NewFixture(tests.FixtureTypeTicker)

			So(len(datapoint.Data), ShouldBeGreaterThan, 0)

			datapoint.InsertReplay(signal.tree, 60, &replaySequence)

			Convey("When I measure each stored ticker row like the trader loop", func() {
				result := measureStoredReplay(signal, signal.tree)

				Convey("It should classify from the ticker replay", func() {
					So(result, ShouldNotBeNil)
					So(categoryResult(result), ShouldBeGreaterThan, 0)
					So(datura.Peek[float64](
						result, "output", "confidence",
					), ShouldBeGreaterThan, 0)
				})
			})
		})
	})
}

func TestCoiledTickerSpread(testingTB *testing.T) {
	Convey("Given a coiled ticker frame through the ignition pipeline", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()
		result := measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should publish a non-zero spread sample", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "spread"), ShouldBeGreaterThan, 0)
			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		result := measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if categoryResult(result) <= 0 {
			b.Fatal("Measure did not classify coiled compression")
		}

		_ = signal.Close()
	}
}
