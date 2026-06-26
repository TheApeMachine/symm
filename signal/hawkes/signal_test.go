package hawkes

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
)

var hawkesCategories = []logic.CategoryType{
	logic.CategoryFrenzy,
	logic.CategorySaturation,
	logic.CategoryOrganic,
	logic.CategoryExhaustion,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, hawkesCategories)
}

func tradeDatapoint(symbol, side string, price, quantity float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":%q,"side":%q,"price":%g,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		symbol, side, price, quantity,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("trade")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func bookDatapoint(bidQty, askQty float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"ALT/EUR","bids":[{"price":100.0,"qty":%g}],"asks":[{"price":101.0,"qty":%g}]}]}`,
		bidQty, askQty,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

var hawkesClassifierInputs = []string{"frenzy", "saturation", "organic", "exhaustion"}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := hawkesClassifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range hawkesClassifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
}

func measureStored(signal *Signal, datapoint *datura.Artifact) *datura.Artifact {
	measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))
	signal.tree = testutil.StoreMeasurement(signal.tree, measured)

	return measured
}

func warmupBalancedTradePairs(signal *Signal, base time.Time, pairCount int, interval time.Duration) time.Time {
	for index := range pairCount {
		at := base.Add(time.Duration(index) * interval).UnixNano()

		for _, side := range []string{"sell", "buy"} {
			frame := tradeDatapoint("ALT/EUR", side, 1, 2, at)
			measured := measureStored(signal, frame)
			if measured != nil {
				measured.Release()
			}
			frame.Release()
		}
	}

	return base.Add(time.Duration(pairCount) * interval)
}

func measureBuyBurstWithBook(
	signal *Signal,
	burstStart time.Time,
	tradeCount int,
	burstInterval time.Duration,
	quantity float64,
	warmInterval time.Duration,
) *datura.Artifact {
	clusterFactor := float64(warmInterval) / float64(burstInterval)

	if clusterFactor < 1 {
		clusterFactor = 1
	}

	bidQty := quantity * clusterFactor
	askQty := quantity

	var (
		result         *datura.Artifact
		bestFrenzy     float64
		bestConfidence float64
	)

	for index := range tradeCount {
		at := burstStart.Add(time.Duration(index) * burstInterval).UnixNano()
		book := bookDatapoint(bidQty, askQty, at)
		_ = testutil.FirstMeasured(signal.Measure(book, nil))
		book.Release()

		frame := tradeDatapoint("ALT/EUR", "buy", 1+float64(index)*0.001, quantity, at)
		var measured *datura.Artifact

		for range 4 {
			candidate := measureStored(signal, frame)

			if candidate != nil {
				if measured != nil {
					measured.Release()
				}

				measured = candidate
			}
		}

		if measured != nil {
			frenzyScore := outputScore(measured, "frenzy")
			confidence := outputScore(measured, "confidence")
			keep := frenzyScore > bestFrenzy

			if frenzyScore == bestFrenzy && confidence > bestConfidence {
				keep = true
			}

			if keep {
				if result != nil {
					result.Release()
				}

				result = measured
				bestFrenzy = frenzyScore
				bestConfidence = confidence
			}

			if !keep {
				measured.Release()
			}
		}

		frame.Release()
	}

	return result
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given a one-sided buy burst with bid-side book support", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		warmInterval := 200 * time.Millisecond
		warmPairs := 128
		eventCount := warmPairs * 2
		minEvents := int(math.Ceil(math.Sqrt(float64(eventCount))))
		burstInterval := warmInterval / time.Duration(minEvents)
		quantity := 2.0

		burstStart := warmupBalancedTradePairs(signal, base, warmPairs, warmInterval)

		result := measureBuyBurstWithBook(
			signal,
			burstStart,
			warmPairs,
			burstInterval,
			quantity,
			warmInterval,
		)

		Convey("It should classify frenzy with the frenzy score winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryFrenzy))
			So(outputScore(result, "frenzy"), ShouldBeGreaterThan, outputScore(result, "saturation"))
			So(outputScore(result, "frenzy"), ShouldBeGreaterThan, outputScore(result, "organic"))
			So(winningClassifierInput(result), ShouldEqual, "frenzy")
			So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})
}

func TestIntensityOfPerSecondUnits(testingTB *testing.T) {
	Convey("Given trade stamps one second apart in nanosecond epochs", testingTB, func() {
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		stamps := make([]float64, 10)

		for index := range stamps {
			stamps[index] = float64(base.Add(time.Duration(index) * time.Second).UnixNano())
		}

		Convey("intensityOf reads ~1 event/second, not ~1e-9/nanosecond", func() {
			lambda := intensityOf(stamps)

			// 10 events spanning 9 seconds = 10/9 ≈ 1.11 per second.
			So(lambda, ShouldAlmostEqual, 10.0/9.0, 0.0001)
			So(lambda, ShouldBeGreaterThan, 0.1)
			So(lambda, ShouldBeLessThan, 1000)
		})
	})
}

func TestSignalColdStartRebuildsFromTree(testingTB *testing.T) {
	Convey("Given prior trade measurements written to a shared tree", testingTB, func() {
		tree := dmt.NewTree("")
		warm := NewSignal(context.Background(), tree)

		defer func() {
			_ = warm.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		for index := range 8 {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			at := base.Add(time.Duration(index) * 100 * time.Millisecond).UnixNano()
			frame := tradeDatapoint("ALT/EUR", side, 1, 2, at)
			measured := measureStored(warm, frame)

			if measured != nil {
				tree = warm.tree
				measured.Release()
			}

			frame.Release()
		}

		Convey("A fresh Signal with empty in-memory state rebuilds from the tree", func() {
			cold := NewSignal(context.Background(), tree)

			defer func() {
				_ = cold.Close()
			}()

			buys, sells, baseline := cold.history("ALT/EUR")

			So(len(buys), ShouldBeGreaterThan, 0)
			So(len(sells), ShouldBeGreaterThan, 0)
			So(len(baseline), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for index := range 32 {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			at := base.Add(time.Duration(index) * 100 * time.Millisecond).UnixNano()
			frame := tradeDatapoint("ALT/EUR", side, 1, 1, at)
			measured := measureStored(signal, frame)
			if measured != nil {
				measured.Release()
			}
			frame.Release()
		}

		_ = signal.Close()
	}
}
