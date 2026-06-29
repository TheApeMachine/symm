package toxicity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
	bookfixtures "github.com/theapemachine/symm/tests/fixtures/book"
	levelfixtures "github.com/theapemachine/symm/tests/fixtures/level3"
)

var toxicityCategories = []logic.CategoryType{
	logic.CategoryToxicBluff,
	logic.CategoryLiquidityVacuum,
	logic.CategoryHardSupport,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, toxicityCategories)
}

var classifierInputs = []string{"bluffScore", "vacuumScore", "supportScore"}

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

func bookFrame(bidQty, askQty float64) string {
	return fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":%g}],"asks":[{"price":101.0,"qty":%g}]}]}`,
		bidQty, askQty,
	)
}

func bookWarmupFrames(bidQty, askQty float64, count int) []string {
	frame := bookFrame(bidQty, askQty)
	frames := make([]string, count)

	for index := range frames {
		frames[index] = frame
	}

	return frames
}

func measureBookFramesForScore(signal *Signal, frames []string, scoreKey string) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestScore      float64
		bestConfidence float64
	)

	for _, frame := range frames {
		datapoint := bookDatapoint(frame)
		measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))

		if measured != nil {
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			score := outputScore(measured, scoreKey)
			confidence := datura.Peek[float64](measured, "output", "confidence")

			if score <= 0 {
				measured.Release()

				datapoint.Release()

				continue
			}

			if score > bestScore || (score == bestScore && confidence > bestConfidence) {
				if result != nil {
					result.Release()
				}

				result = measured
				bestScore = score
				bestConfidence = confidence

				datapoint.Release()

				continue
			}

			measured.Release()
		}

		datapoint.Release()
	}

	return result
}

func bookDatapoint(payload string) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	bookReplaySequence += int64(time.Millisecond)
	artifact.SetTimestamp(bookReplaySequence)

	return artifact
}

var bookReplaySequence = time.Now().UnixNano()

const bookQualityWarmupFrames = 3

func bookQualitySupportFrames() []string {
	frames := bookWarmupFrames(10, 10, bookQualityWarmupFrames)
	frames = append(frames,
		bookFrame(10, 10),
		bookFrame(12, 12),
		bookFrame(14, 14),
		bookFrame(16, 16),
		bookFrame(18, 18),
		bookFrame(20, 20),
	)

	return frames
}

// l3Order renders one per-order event for an L3 side array.
func l3Order(event string, price, qty float64) string {
	return fmt.Sprintf(
		`{"event":%q,"order_id":"O-%g-%g","limit_price":%g,"order_qty":%g,"timestamp":"2026-05-30T12:00:00Z"}`,
		event, price, qty, price, qty,
	)
}

// insertLevel3 writes one level3 ingest frame (role=level3) into the signal's
// tree at the given stamp, exactly as kraken/public/websocket.go persists it.
func insertLevel3(signal *Signal, bids, asks []string, stamp int64) {
	payload := fmt.Sprintf(
		`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD","bids":[%s],"asks":[%s]}]}`,
		strings.Join(bids, ","), strings.Join(asks, ","),
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("level3")
	artifact.WithScope("BTC/USD")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(stamp)
	signal.tree, _, _ = signal.tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
	signal.tree, _, _ = signal.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)
}

func insertTrade(signal *Signal, symbol string, price float64, stamp int64) {
	payload := fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":%q,"side":"buy","price":%g,"qty":1,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		symbol, price,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("trade")
	artifact.WithScope(symbol)
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(stamp)
	signal.tree, _, _ = signal.tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
	signal.tree, _, _ = signal.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)
}

// warmBook drives a few quiet book frames so the tree carries measurement
// history (cancel baseline + window stamps) for the final scored frame.
func warmBook(signal *Signal, count int) {
	for index := 0; index < count; index++ {
		datapoint := bookDatapoint(bookFrame(10, 10))
		measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))

		if measured != nil {
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			measured.Release()
		}

		datapoint.Release()
	}
}

func measureL3Bluff(signal *Signal) *datura.Artifact {
	warmBook(signal, 4)
	stamp := bookReplaySequence + int64(time.Microsecond)

	for index := 0; index < 6; index++ {
		insertLevel3(signal,
			[]string{l3Order("delete", 100, 40)},
			[]string{l3Order("delete", 101, 40)},
			stamp+int64(index)*int64(time.Microsecond),
		)
	}

	bookReplaySequence = stamp + 6*int64(time.Microsecond)
	datapoint := bookDatapoint(bookFrame(10, 10))

	defer datapoint.Release()

	return testutil.FirstMeasured(signal.Measure(datapoint, nil))
}

func measureL3Vacuum(signal *Signal) *datura.Artifact {
	warmBook(signal, 4)
	stamp := bookReplaySequence + int64(time.Microsecond)

	for index := 0; index < 6; index++ {
		insertLevel3(signal,
			[]string{l3Order("delete", 100, 90)},
			nil,
			stamp+int64(index)*int64(time.Microsecond),
		)
	}

	bookReplaySequence = stamp + 6*int64(time.Microsecond)
	datapoint := bookDatapoint(bookFrame(10, 10))

	defer datapoint.Release()

	return testutil.FirstMeasured(signal.Measure(datapoint, nil))
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given L3 deletes with no trades (symmetric cancels)", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		// Bluff is an L3 story: near-touch blocks delete WITHOUT a coincident
		// trade (pulled, not hit) and both sides pull, so asymmetry is low. L2
		// qty deltas cannot derive this, so it is driven from level3 ingest.
		result := measureL3Bluff(signal)

		Convey("It should classify toxic bluff with bluffScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "bluffScore"), ShouldBeGreaterThan, outputScore(result, "vacuumScore"))
			So(outputScore(result, "bluffScore"), ShouldBeGreaterThan, outputScore(result, "supportScore"))
			So(winningClassifierInput(result), ShouldEqual, "bluffScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryToxicBluff))

			result.Release()
		})
	})

	Convey("Given one-sided L3 deletes with no trades", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		// Vacuum is an L3 story: one side cancels aggressively (high asymmetry).
		result := measureL3Vacuum(signal)

		Convey("It should classify liquidity vacuum with vacuumScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "vacuumScore"), ShouldBeGreaterThan, outputScore(result, "bluffScore"))
			So(outputScore(result, "vacuumScore"), ShouldBeGreaterThan, outputScore(result, "supportScore"))
			So(winningClassifierInput(result), ShouldEqual, "vacuumScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLiquidityVacuum))

			result.Release()
		})
	})

	Convey("Given a warmed sincere support book", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := bookQualitySupportFrames()
		result := measureBookFramesForScore(signal, frames, "supportScore")

		Convey("It should classify hard support with supportScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "supportScore"), ShouldBeGreaterThan, outputScore(result, "bluffScore"))
			So(outputScore(result, "supportScore"), ShouldBeGreaterThan, outputScore(result, "vacuumScore"))
			So(winningClassifierInput(result), ShouldEqual, "supportScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryHardSupport))

			result.Release()
		})
	})
}

func TestSignalColdStartRebuildsFromTree(testingTB *testing.T) {
	Convey("Given prior measurements written to a shared tree", testingTB, func() {
		tree := dmt.NewTree("")
		warm := NewSignal(context.Background(), tree)

		defer func() {
			_ = warm.Close()
		}()

		// Build cancel + touch history on one Signal, persisting each measurement
		// to the tree (the ONLY history store — no sync.Map backs the Signal).
		for index := 0; index < 5; index++ {
			datapoint := bookDatapoint(bookFrame(10+float64(index), 10+float64(index)))
			measured := testutil.FirstMeasured(warm.Measure(datapoint, nil))

			if measured != nil {
				tree = testutil.StoreMeasurement(tree, measured)
				measured.Release()
			}

			datapoint.Release()
		}

		Convey("A fresh Signal with empty in-memory state rebuilds from the tree", func() {
			cold := NewSignal(context.Background(), tree)

			defer func() {
				_ = cold.Close()
			}()

			cancelHistory, windowStamps, prevBidQty, prevAskQty := cold.history("BTC/USD")

			So(len(cancelHistory), ShouldBeGreaterThan, 0)
			So(len(windowStamps), ShouldBeGreaterThan, 0)
			So(prevBidQty, ShouldBeGreaterThan, 0)
			So(prevAskQty, ShouldBeGreaterThan, 0)

			datapoint := bookDatapoint(bookFrame(2, 2))

			defer datapoint.Release()

			result := testutil.FirstMeasured(cold.Measure(datapoint, nil))

			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

			result.Release()
		})
	})
}

func TestScopedIngestReplayUsesSymbolIndex(testingTB *testing.T) {
	Convey("Given trade frames for multiple symbols", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		stamp := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

		defer func() {
			_ = signal.Close()
		}()

		insertTrade(signal, "BTC/USD", 100, stamp)
		insertTrade(signal, "ETH/USD", 200, stamp+1)

		Convey("tradePrices should seek the scoped tree index for the requested symbol only", func() {
			prices := signal.tradePrices("BTC/USD", 0, float64(stamp+2))

			So(prices, ShouldResemble, []float64{100})
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a warmed book replay", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := []string{
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":1.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		}

		var (
			result         *datura.Artifact
			bestConfidence float64
		)

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)

				if !testutil.HasConfidence(measured) {
					measured.Release()

					datapoint.Release()

					continue
				}

				confidence := datura.Peek[float64](measured, "output", "confidence")

				if confidence > bestConfidence {
					result = measured
					bestConfidence = confidence
				}
			}

			datapoint.Release()
		}

		Convey("It returns classifier output with non-uniform confidence", func() {
			So(result, ShouldNotBeNil)

			role, _ := result.Role()
			scope, _ := result.Scope()

			So(role, ShouldEqual, "measurement")
			So(scope, ShouldEqual, "BTC/USD")
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 0)
			So(testutil.DistributionSum(result, toxicityCategories), ShouldAlmostEqual, 1, 0.0001)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(bestConfidence, ShouldNotAlmostEqual, 1.0/3.0, 0.0001)

			result.Release()
		})
	})

	Convey("Given a single book frame without warmup", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		datapoint := bookDatapoint(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`)

		defer datapoint.Release()

		result := testutil.FirstMeasured(signal.Measure(datapoint, nil))

		Convey("It should publish on the first book observation", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestMeasureBookFrames(testingTB *testing.T) {
	Convey("Given live-shaped kraken book fixtures", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		signal := NewSignal(ctx, dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		var result *datura.Artifact

		frames := []string{
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":1.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		}

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)

				if testutil.HasConfidence(measured) {
					result = measured
				}
			}

			datapoint.Release()
		}

		Convey("It should emit classifier output on a writable measurement artifact", func() {
			So(result, ShouldNotBeNil)
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 2)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

			result.Release()
		})
	})
}

func TestMeasureKrakenFixtureStreamUsesLevel3(testingTB *testing.T) {
	Convey("Given Kraken book and level3 fixture streams", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		signal := NewSignal(ctx, dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		base := time.Now().UTC().Truncate(time.Second)
		index := 0

		for artifact := range levelfixtures.NewFixture(levelfixtures.UPDATE, 3).Artifacts() {
			artifact.WithScope("MATIC/USD")
			artifact.SetTimestamp(base.Add(time.Duration(index) * time.Second).UnixNano())
			signal.tree.InsertArtifact(artifact.Prefix("role", "scope", "timestamp"), artifact)
			artifact.Release()
			index++
		}

		var result *datura.Artifact

		for artifact := range bookfixtures.NewFixture(bookfixtures.SNAPSHOT, 1).Artifacts() {
			artifact.SetTimestamp(base.Add(10 * time.Second).UnixNano())
			result = testutil.FirstMeasured(signal.Measure(artifact, nil))
			artifact.Release()
		}

		Convey("When toxicity measures the book frame", func() {
			So(result, ShouldNotBeNil)

			Convey("Then it should use the level3 order-event basis", func() {
				So(datura.Peek[float64](result, "output", "l3"), ShouldEqual, 1)
				So(datura.Peek[float64](result, "output", "cancelTotal"), ShouldBeGreaterThan, 0)

				result.Release()
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	frames := []string{
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
	}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		var result *datura.Artifact

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)

				if testutil.HasConfidence(measured) {
					result = measured
				}
			}

			datapoint.Release()
		}

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		result.Release()
		_ = signal.Close()
	}
}
