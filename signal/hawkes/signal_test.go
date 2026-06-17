package hawkes

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
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

func encodeFloatPayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func excitationBurstSamples(base time.Time, count int) []float64 {
	buyTimes := make([]float64, 0, count/2)
	sellTimes := make([]float64, 0, count/2)

	for index := range count {
		wall := base.Add(time.Duration(index) * 100 * time.Millisecond)
		seconds := float64(wall.UnixNano()) / float64(time.Second)

		if index%2 == 0 {
			sellTimes = append(sellTimes, seconds)
			continue
		}

		buyTimes = append(buyTimes, seconds)
	}

	horizon := float64(base.Add(time.Duration(count)*100*time.Millisecond).UnixNano()) / float64(time.Second)
	span := base.Add(time.Duration(count) * 100 * time.Millisecond).Sub(base)
	cooldown := algorithm.DeriveFitCooldown(span).Seconds()

	samples := []float64{
		horizon,
		cooldown,
		float64(len(buyTimes)),
		float64(len(sellTimes)),
	}
	samples = append(samples, buyTimes...)
	samples = append(samples, sellTimes...)

	return samples
}

func insertTradeExcitation(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("trade")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	Convey("Given a clustered trade burst", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertTradeExcitation(signal, "ALT/EUR", excitationBurstSamples(base, 128)...)

		result := signal.Measure(measurementQuery("ALT/EUR"))

		Convey("It should produce a publishable thermal reading", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ALT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)

			var indexed bool

			for inbound := range signal.tree.Seek(result.Prefix()) {
				indexed = true
				inbound.Release()
			}

			So(indexed, ShouldBeTrue)
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		result := signal.Measure(measurementQuery("NEW/EUR"))

		Convey("It should return nil without error", func() {
			So(result, ShouldBeNil)
		})
	})
}

func TestSignalMeasureRejectsKrakenJSONTreeRows(testingTB *testing.T) {
	Convey("Given Kraken trade JSON indexed in the shared tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		row := datura.Acquire("kraken", datura.Artifact_Type_json)
		row.WithRole("trade")
		row.WithScope("BTC/EUR")
		So(row.From(krakenmarket.TradeUpdates{{
			Symbol: "BTC/EUR",
			Price:  50000,
			Qty:    0.1,
			Side:   "buy",
		}}), ShouldBeNil)

		signal.tree.Insert(row.Prefix(), row.Marshal())
		row.Release()

		Convey("It should skip malformed replay rows without panicking", func() {
			result := signal.Measure(measurementQuery("BTC/EUR"))
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	query := measurementQuery("BTC/EUR")
	payload := excitationBurstSamples(base, 128)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertTradeExcitation(signal, "BTC/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		_ = signal.Close()
	}
}
