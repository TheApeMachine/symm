package pumpdump

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/tests"
)

const pumpdumpWarmupTicks = 59

func categoryResult(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

var classifierInputs = []string{"ignition", "compression", "trend", "exhaustion"}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := classifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range classifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
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
		warmupLast := last + float64(tick)*0.1
		result = measureTickerFrame(signal, symbol, volume, vwap, warmupLast, bid, ask, changePct)
	}

	return result
}

func measureStoredReplay(signal *Signal, tree *dmt.Tree) *datura.Artifact {
	var storedRows []*datura.Artifact

	for stored := range tree.Seek([]byte(tickerUpdatePrefix)) {
		storedRows = append(storedRows, stored)
	}

	sort.Slice(storedRows, func(left, right int) bool {
		return storedRows[left].Timestamp() < storedRows[right].Timestamp()
	})

	var result *datura.Artifact

	for _, stored := range storedRows {
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
	// Warmup uses 59 ticks; one more steady tick follows.
	return 6100, 10000, 10060, 10020, 10040, 0.35
}

func fadedExhaustionTicker() (float64, float64, float64, float64, float64, float64) {
	// Warmup adds 200 per cumulative tick; a 1 increment is sharply fading lift.
	return 11801, 10000, 10100, 10080, 10120, 0.05
}

func TestSignalMeasureCategorySemantics(t *testing.T) {
	Convey("Given a warmed vertical ignition ticker", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
		warmupTickerFrames(signal, "ETH/EUR", pumpdumpWarmupTicks, 100, vwap, 10000, 9990, 10010, 0)
		result := measureTickerFrame(signal, "ETH/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should show high lift and precursor with ignition winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "rvol"), ShouldBeGreaterThan, 3)
			So(outputScore(result, "precursor"), ShouldBeGreaterThan, 1)
			So(outputScore(result, "ignition"), ShouldBeGreaterThan, outputScore(result, "compression"))
			So(winningClassifierInput(result), ShouldEqual, "ignition")
			So(categoryResult(result), ShouldEqual, 1)
		})
	})

	Convey("Given a warmed coiled compression ticker", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()

		for tick := range pumpdumpWarmupTicks {
			volumeStep := 120.0 * float64(tick+1)
			warmupLast := 10050.0 + float64(tick)*0.1
			warmupResult := measureTickerFrame(
				signal, "BTC/EUR", volumeStep, vwap, warmupLast, 10040, 10060, 0,
			)
			warmupResult.Release()
		}

		result := measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should show moderate lift, low precursor, and compression winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "rvol"), ShouldBeGreaterThan, 1)
			So(outputScore(result, "rvol"), ShouldBeLessThan, 2)
			So(outputScore(result, "precursor"), ShouldAlmostEqual, 0, 0.0001)
			So(outputScore(result, "compression"), ShouldBeGreaterThan, outputScore(result, "ignition"))
			So(outputScore(result, "spread"), ShouldBeGreaterThan, 0)
			So(winningClassifierInput(result), ShouldEqual, "compression")
			So(categoryResult(result), ShouldEqual, 2)
		})
	})

	Convey("Given a warmed organic trend ticker", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := organicTrendTicker()

		for tick := range pumpdumpWarmupTicks {
			volumeStep := 100.0 * float64(tick+1)
			warmupLast := 10000.0 + float64(tick)*0.5
			warmupResult := measureTickerFrame(
				signal, "TREND/EUR", volumeStep, vwap, warmupLast, 10020, 10040, 0.15,
			)
			warmupResult.Release()
		}

		result := measureTickerFrame(signal, "TREND/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should show steady lift, moderate precursor, and trend winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "rvol"), ShouldBeGreaterThan, 0.8)
			So(outputScore(result, "rvol"), ShouldBeLessThan, 1.5)
			So(outputScore(result, "precursor"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "trend"), ShouldBeGreaterThan, outputScore(result, "ignition"))
			So(winningClassifierInput(result), ShouldEqual, "trend")
			So(categoryResult(result), ShouldEqual, 3)
		})
	})

	Convey("Given a warmed faded exhaustion ticker", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := fadedExhaustionTicker()
		warmupTickerFrames(signal, "FADE/EUR", pumpdumpWarmupTicks, 200, vwap, 10100, 10070, 10130, 0.05)
		result := measureTickerFrame(signal, "FADE/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should show declining lift, flat precursor, and exhaustion winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "rvol"), ShouldBeLessThan, 1)
			So(outputScore(result, "rvolDecline"), ShouldBeGreaterThan, 0.5)
			So(outputScore(result, "precursor"), ShouldAlmostEqual, 0, 0.0001)
			So(outputScore(result, "exhaustion"), ShouldBeGreaterThan, outputScore(result, "ignition"))
			So(winningClassifierInput(result), ShouldEqual, "exhaustion")
			So(categoryResult(result), ShouldEqual, 4)
		})
	})
}

func TestSignalMeasureFlatLastDoesNotStall(t *testing.T) {
	Convey("Given a warmed signal and repeated flat last prices", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmupTickerFrames(signal, "FLAT/EUR", pumpdumpWarmupTicks, 100, 10000, 10000, 9990, 10010, 0)

		var result *datura.Artifact

		for tick := range 30 {
			volume := 6000.0 + float64(tick)*10
			next := measureTickerFrame(signal, "FLAT/EUR", volume, 10000, 10000, 9990, 10010, 0)

			if result != nil {
				result.Release()
			}

			result = next
		}

		if result != nil {
			defer result.Release()
		}

		Convey("It should keep measuring through zero-variance log returns", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "rvol"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureColdStartUsesFirstPrior(t *testing.T) {
	Convey("Given a fresh signal and a single ticker frame", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()
		result := measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should seed calibration from the first observation", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
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
		warmupTickerFrames(signal, "ETH/EUR", pumpdumpWarmupTicks, 100, vwap, 10000, 9990, 10010, 0)
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
			So(datura.Peek[float64](result, "output", "confidence"), ShouldNotAlmostEqual, 0.25, 0.0001)
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

		for tick := range pumpdumpWarmupTicks {
			volumeStep := 120.0 * float64(tick+1)
			warmupLast := 10050.0 + float64(tick)*0.1
			result = measureTickerFrame(
				signal, "BTC/EUR", volumeStep, vwap, warmupLast, 10040, 10060, 0,
			)
			result.Release()
		}

		result = measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should classify coiled compression from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 2)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldNotAlmostEqual, 0.25, 0.0001)
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

		for tick := range pumpdumpWarmupTicks {
			volumeStep := 100.0 * float64(tick+1)
			warmupLast := 10000.0 + float64(tick)*0.5
			result = measureTickerFrame(
				signal, "TREND/EUR", volumeStep, vwap, warmupLast, 10020, 10040, 0.15,
			)
			result.Release()
		}

		result = measureTickerFrame(signal, "TREND/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should classify organic trend from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 3)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldNotAlmostEqual, 0.25, 0.0001)
		})
	})

	Convey("Given fading volume lift with flat precursor", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := fadedExhaustionTicker()
		warmupTickerFrames(signal, "FADE/EUR", pumpdumpWarmupTicks, 200, vwap, 10100, 10070, 10130, 0.05)
		result := measureTickerFrame(signal, "FADE/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should classify faded exhaustion from the ticker replay", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 4)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldNotAlmostEqual, 0.25, 0.0001)
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

func TestSignalMeasureRejectsNonTickerChannel(t *testing.T) {
	Convey("Given a book ingest frame", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		datapoint := tests.NewFixture(tests.FixtureTypeBook).ToArtifact()
		So(datapoint, ShouldNotBeNil)

		defer datapoint.Release()

		result := signal.Measure(datapoint)

		Convey("It should not emit a measurement", func() {
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

		warmupTickerFrames(signal, "REPLAY/USD", pumpdumpWarmupTicks, 100, 10000, 10000, 9990, 10010, 0)

		volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
		result := measureTickerFrame(signal, "REPLAY/USD", volume, vwap, last, bid, ask, changePct)

		Convey("It should complete replay without losing classifier output", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func insertTickerReplay(
	tree *dmt.Tree,
	symbol string,
	tickCount int,
	volumeStep, vwap, last, bid, ask, changePct float64,
) {
	for tick := range tickCount {
		volume := volumeStep * float64(tick+1)
		tickLast := last + float64(tick)*0.1
		stored := datura.Acquire("kraken:public", datura.APPJSON)
		stored.WithRole("ticker")
		stored.WithScope("update")
		stored.WithPayload(krakenTickerFrame(volume, vwap, tickLast, bid, ask, changePct, symbol))
		replaySequence++
		stored.SetTimestamp(replaySequence)
		tree.Insert(stored.Prefix(), stored.Pack())
		stored.Release()
	}
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

		Convey("And a warmed ticker replay in the tree", func() {
			insertTickerReplay(
				signal.tree, "REPLAY/USD", pumpdumpWarmupTicks,
				100, 10000, 10000, 9990, 10010, 0,
			)

			volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
			insertTickerReplay(
				signal.tree, "REPLAY/USD", 1,
				volume, vwap, last, bid, ask, changePct,
			)

			Convey("When I measure each stored ticker row like the trader loop", func() {
				result := measureStoredReplay(signal, signal.tree)

				Convey("It should classify vertical ignition from the replay", func() {
					So(result, ShouldNotBeNil)
					So(categoryResult(result), ShouldEqual, 1)
					So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0)
					So(outputScore(result, "confidence"), ShouldNotAlmostEqual, 0.25, 0.0001)
				})
			})
		})
	})
}

func TestCoiledTickerSpread(testingTB *testing.T) {
	Convey("Given a warmed coiled ticker frame through the ignition pipeline", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		volume, vwap, last, bid, ask, changePct := coiledCompressionTicker()

		for tick := range pumpdumpWarmupTicks {
			volumeStep := 120.0 * float64(tick+1)
			warmupLast := 10050.0 + float64(tick)*0.1
			warmupResult := measureTickerFrame(
				signal, "BTC/EUR", volumeStep, vwap, warmupLast, 10040, 10060, 0,
			)
			warmupResult.Release()
		}

		result := measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		Convey("It should publish a non-zero spread sample", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "spread"), ShouldBeGreaterThan, 0)
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

		for tick := range pumpdumpWarmupTicks {
			volumeStep := 120.0 * float64(tick+1)
			warmupLast := 10050.0 + float64(tick)*0.1
			warmupResult := measureTickerFrame(
				signal, "BTC/EUR", volumeStep, vwap, warmupLast, 10040, 10060, 0,
			)

			if warmupResult != nil {
				warmupResult.Release()
			}
		}

		result := measureTickerFrame(signal, "BTC/EUR", volume, vwap, last, bid, ask, changePct)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if categoryResult(result) != 2 {
			b.Fatalf("Measure classified category %d, want coiled compression (2)", categoryResult(result))
		}

		if math.Abs(outputScore(result, "confidence")-0.25) < 1e-4 {
			b.Fatal("Measure returned uniform confidence")
		}

		result.Release()
		_ = signal.Close()
	}
}
