package manifold

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/signal/testutil"
)

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func insertTreeArtifact(signal *Signal, role, scope string, payload []byte) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire := artifact.Pack(); len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func storeManifoldMeasurement(signal *Signal, measurement *datura.Artifact) {
	if measurement == nil {
		return
	}

	signal.tree = testutil.StoreMeasurement(signal.tree, measurement)
}

func insertManifoldFeaturePayload(signal *Signal, scope string, samples []float64) {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	artifact := datura.Acquire("manifold-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire := artifact.Pack(); len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func setManifoldTestViper() {
	viper.Set("signals.manifold.measurements_capacity", 16)
	viper.Set("signals.manifold.tick_size", 0.01)
	viper.Set("signals.manifold.grid_half_width", 8)
	viper.Set("signals.manifold.grid_x", 16)
	viper.Set("signals.manifold.grid_y", 1)
	viper.Set("signals.manifold.grid_z", 8)
	viper.Set("signals.manifold.max_modes", 8)
	viper.Set("signals.manifold.integration_interval", "100ms")
	viper.Set("market.book_depth_levels", 4)
}

func TestHydrateFieldFromTree(t *testing.T) {
	Convey("Given book trade and ticker tree rows", t, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		signal.field.RegisterSymbols([]string{"XBT/USD"})

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		bookRaw, bookErr := json.Marshal(BookUpdate{
			Symbol:    "XBT/USD",
			Type:      "snapshot",
			Timestamp: eventAt,
			Bids:      []BookLevel{{Price: 49990, Qty: 1}},
			Asks:      []BookLevel{{Price: 50010, Qty: 1}},
		})

		So(bookErr, ShouldBeNil)
		insertTreeArtifact(signal, "book", "XBT/USD", bookRaw)

		tradeRaw, tradeErr := json.Marshal([]TradeUpdate{{
			Symbol:    "XBT/USD",
			Price:     50000,
			Qty:       0.2,
			Side:      "buy",
			Timestamp: eventAt,
		}})

		So(tradeErr, ShouldBeNil)
		insertTreeArtifact(signal, "trade", "XBT/USD", tradeRaw)

		tickerRaw, tickerErr := json.Marshal([]TickerUpdate{{
			Symbol:    "XBT/USD",
			Last:      50000,
			Bid:       49990,
			Ask:       50010,
			BidQty:    1,
			AskQty:    1,
			Timestamp: eventAt,
		}})

		So(tickerErr, ShouldBeNil)
		insertTreeArtifact(signal, "ticker", "XBT/USD", tickerRaw)

		signal.hydrateFieldFromTree()
		time.Sleep(150 * time.Millisecond)

		state := signal.field.universe.loadSymbol("XBT/USD")

		Convey("It should hydrate the field from tree ingest prefixes", func() {
			So(state, ShouldNotBeNil)
			So(state.bookReady, ShouldBeTrue)
			So(state.midPrice, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given Kraken wire envelopes in the tree", t, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		signal.field.RegisterSymbols([]string{"BTC/USD"})

		tickerWire := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":50000,"bid":49990,"ask":50010,"timestamp":"2024-01-01T00:00:00Z"}]}`)
		insertTreeArtifact(signal, "ticker", "update", tickerWire)

		signal.hydrateFieldFromTree()
		time.Sleep(150 * time.Millisecond)

		state := signal.field.universe.loadSymbol("BTC/USD")

		Convey("It should hydrate from Kraken data envelopes", func() {
			So(state, ShouldNotBeNil)
			So(state.lastPrice, ShouldEqual, 50000)
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given herd manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "HERD/EUR", herdManifoldPayload())

		result := signal.Measure(measurementQuery("HERD/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify herd and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "HERD/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "HERD/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given shock manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "SHOCK/EUR", shockManifoldPayload())

		result := signal.Measure(measurementQuery("SHOCK/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify shock and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "SHOCK/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "SHOCK/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given drift manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "DRIFT/EUR", driftManifoldPayload())

		result := signal.Measure(measurementQuery("DRIFT/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify drift and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "DRIFT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "DRIFT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given noise manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "NOISE/EUR", noiseManifoldPayload())

		result := signal.Measure(measurementQuery("NOISE/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify noise and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "NOISE/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "NOISE/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("NEW/EUR"), nil)

		Convey("It should return nil without halting", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NEW/EUR"), ShouldBeFalse)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given herd manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "HERD/EUR", herdManifoldPayload())

		result := signal.Measure(measurementQuery("HERD/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify herd and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "HERD/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "HERD/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given shock manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "SHOCK/EUR", shockManifoldPayload())

		result := signal.Measure(measurementQuery("SHOCK/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify shock and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "SHOCK/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "SHOCK/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given drift manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "DRIFT/EUR", driftManifoldPayload())

		result := signal.Measure(measurementQuery("DRIFT/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify drift and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "DRIFT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "DRIFT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given noise manifold features", testingTB, func() {
		setManifoldTestViper()

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertManifoldFeaturePayload(signal, "NOISE/EUR", noiseManifoldPayload())

		result := signal.Measure(measurementQuery("NOISE/EUR"), nil)
		storeManifoldMeasurement(signal, result)

		Convey("It should classify noise and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "NOISE/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "NOISE/EUR"), ShouldBeTrue)
			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	setManifoldTestViper()

	query := measurementQuery("HERD/EUR")
	payload := herdManifoldPayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertManifoldFeaturePayload(signal, "HERD/EUR", payload)

		result := signal.Measure(query, nil)
		storeManifoldMeasurement(signal, result)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "HERD/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/HERD/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
