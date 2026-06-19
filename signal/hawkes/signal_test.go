package hawkes

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
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

func newInstrumentedSignal(testingTB testing.TB, tree *dmt.Tree) (*Signal, *algorithm.Excitation) {
	if testingTB != nil {
		testingTB.Helper()
	}

	ctx, cancel := context.WithCancel(context.Background())
	excitation := algorithm.NewExcitation()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        newTestPool(testingTB),
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			excitation,
			probability.NewClassifier(
				datura.Acquire("hawkes-classifier", datura.APPJSON).Poke(
					[]string{"frenzy", "saturation", "organic", "exhaustion"},
					"inputs",
				),
			),
		),
	}

	return signal, excitation
}

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func encodeFloatPayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func insertTradeExcitation(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("trade")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a sparse pre-fit trade window", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertTradeExcitation(signal, "FREN/EUR", frenzyExcitationPayload()...)

		result := signal.Measure(measurementQuery("FREN/EUR"))

		Convey("It should classify frenzy and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "FREN/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "FREN/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given gate-warmed history and a high-radius burst", testingTB, func() {
		signal, excitation := newInstrumentedSignal(testingTB, dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmExcitationScope(excitation, "SAT/EUR",
			saturationGateWarmPayload(),
			saturationGateWarmPayload(),
			saturationGateWarmPayload(),
			saturationGateWarmPayload(),
		)
		insertTradeExcitation(signal, "SAT/EUR", saturationBurstPayload()...)

		result := signal.Measure(measurementQuery("SAT/EUR"))

		Convey("It should classify saturation and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "SAT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "SAT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a clustered trade burst", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertTradeExcitation(signal, "ORG/EUR", organicExcitationPayload()...)

		result := signal.Measure(measurementQuery("ORG/EUR"))

		Convey("It should classify organic flow and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ORG/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ORG/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given gate-warmed history and a faded arrival stream", testingTB, func() {
		signal, excitation := newInstrumentedSignal(testingTB, dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmExcitationScope(excitation, "EXH/EUR",
			shiftedOrganicPayload(0),
			shiftedOrganicPayload(time.Minute),
			shiftedOrganicPayload(2*time.Minute),
			shiftedOrganicPayload(3*time.Minute),
		)
		insertTradeExcitation(signal, "EXH/EUR", exhaustionFadePayload()...)

		result := signal.Measure(measurementQuery("EXH/EUR"))

		Convey("It should classify exhaustion and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "EXH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "EXH/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("NEW/EUR"))

		Convey("It should return nil without error", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NEW/EUR"), ShouldBeFalse)
		})
	})

}

func TestSignalMeasureRejectsKrakenJSONTreeRows(testingTB *testing.T) {
	Convey("Given Kraken trade JSON indexed in the shared tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		row := datura.Acquire("kraken", datura.Artifact_Type_json)
		row.WithRole("trade")
		row.WithScope("BTC/EUR")
		row.WithPayload([]byte(`[{"symbol":"BTC/EUR","price":50000,"qty":0.1,"side":"buy"}]`))

	if wire, err := row.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(row.Prefix(), wire)
	}

	row.Release()

		Convey("It should skip malformed replay rows without panicking", func() {
			result := signal.Measure(measurementQuery("BTC/EUR"))
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("BTC/EUR")
	payload := organicExcitationPayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertTradeExcitation(signal, "BTC/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "BTC/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/BTC/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
