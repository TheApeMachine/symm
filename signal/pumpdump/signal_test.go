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

func insertTickerReplay(
	signal *Signal,
	query *datura.Artifact,
	volume, vwap, last, bid, ask, changePct float64,
) {
	scope, scopeErr := query.Scope()
	role, roleErr := query.Role()

	if scopeErr != nil || roleErr != nil || scope == "" || role == "" {
		return
	}

	replay := datura.Acquire("kraken:public", datura.APPJSON)
	replay.WithRole(role)
	replay.WithScope(scope)
	replay.WithPayload(krakenTickerFrame(volume, vwap, last, bid, ask, changePct, scope))

	// Stamp a strictly-increasing timestamp so replay order is deterministic:
	// Seek replays in timestamp order, and wall-clock nanoseconds collide under
	// a tight pooled-allocation loop, scrambling temporal feature accumulation.
	replaySequence++
	replay.SetTimestamp(replaySequence)
	replay.WithDestination(fmt.Sprintf("%020d", replaySequence))

	wire := replay.Pack()

	if len(wire) == 0 {
		replay.Release()
		return
	}

	signal.tree.Insert(replay.Prefix(), wire)
	replay.Release()
}

func insertTickerWarmup(
	signal *Signal,
	query *datura.Artifact,
	tickCount int,
	volumeStep, vwap, last, bid, ask, changePct float64,
) {
	for tick := range tickCount {
		volume := volumeStep * float64(tick+1)
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)
	}
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

		query := tickerQuery("ETH/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
		insertTickerWarmup(signal, query, 59, 100, vwap, 10000, 9990, 10010, 0)
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

		Convey("It should classify vertical ignition from the ticker replay", func() {
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

		query := tickerQuery("BTC/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()
		insertTickerWarmup(signal, query, 59, 120, vwap, 10050, 10040, 10060, 0)
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

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

		query := tickerQuery("TREND/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := organicTrendTicker()
		insertTickerWarmup(signal, query, 59, 100, vwap, 12400, 12490, 12510, 0.15)
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

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

		query := tickerQuery("FADE/EUR")

		defer query.Release()

		volume, vwap, last, bid, ask, changePct := fadedExhaustionTicker()
		insertTickerWarmup(signal, query, 59, 200, vwap, 10100, 10095, 10105, 0.05)
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

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

		query := tickerQuery("NEW/EUR")

		defer query.Release()

		result := signal.Measure(query)

		Convey("It should leave the query unclassified without ticker rows", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldEqual, 0)
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

		query := tickerQuery("REPLAY/USD")

		defer query.Release()

		insertTickerWarmup(signal, query, 120, 100, 10000, 10000, 9990, 10010, 0)

		volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

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

			query := datapoint.ToArtifact()

			So(query, ShouldNotBeNil)

			defer query.Release()

			datapoint.InsertReplay(signal.tree, 60, &replaySequence)

			Convey("When I measure the ticker datapoint", func() {
				result := signal.Measure(query)

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

func BenchmarkSignalMeasure(b *testing.B) {
	query := tickerQuery("BTC/EUR")
	volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertTickerReplay(signal, query, volume, vwap, last, bid, ask, changePct)

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if categoryResult(result) <= 0 {
			b.Fatal("Measure did not classify coiled compression")
		}

		_ = signal.Close()
	}

	query.Release()
}
