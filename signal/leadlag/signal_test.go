package leadlag

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
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/qpool"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	testingTB.Helper()

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func insertLagFeatures(signal *Signal, scope string, samples []float64) {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	artifact := datura.Acquire("lag-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given inefficient lag features", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertLagFeatures(signal, "LAG/EUR", inefficientLagPayload())

		result := signal.Measure(measurementQuery("LAG/EUR"))

		Convey("It should classify inefficient lag and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "LAG/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "LAG/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given synchronized drift features", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertLagFeatures(signal, "SYNC/EUR", syncDriftPayload())

		result := signal.Measure(measurementQuery("SYNC/EUR"))

		Convey("It should classify synchronized drift and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "SYNC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "SYNC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given decoupled move features", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertLagFeatures(signal, "DEC/EUR", decoupledMovePayload())

		result := signal.Measure(measurementQuery("DEC/EUR"))

		Convey("It should classify decoupled move and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "DEC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "DEC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given anchor stall features", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertLagFeatures(signal, "BTC/EUR", anchorStallPayload())

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify anchor stall and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeTrue)
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

		Convey("It should return nil without halting", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NEW/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("LAG/EUR")
	payload := inefficientLagPayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertLagFeatures(signal, "LAG/EUR", payload)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "LAG/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/LAG/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}

func insertTreeArtifact(signal *Signal, role, scope string, payload []byte) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

type tickerUpdate struct {
	Symbol    string    `json:"symbol"`
	Last      float64   `json:"last"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	Timestamp time.Time `json:"timestamp"`
}

func insertTickerRow(
	signal *Signal,
	symbol string,
	price float64,
	eventAt time.Time,
) {
	raw, err := json.Marshal([]tickerUpdate{{
		Symbol:    symbol,
		Last:      price,
		Bid:       price - 0.01,
		Ask:       price + 0.01,
		Timestamp: eventAt,
	}})

	if err != nil {
		panic(err)
	}

	insertTreeArtifact(signal, "ticker", symbol, raw)
}

func seedTickerSeries(
	signal *Signal,
	symbol string,
	start time.Time,
	count int,
	spacing time.Duration,
	priceFn func(index int) float64,
) {
	for index := range count {
		insertTickerRow(signal, symbol, priceFn(index), start.Add(time.Duration(index)*spacing))
	}
}

func TestSectionPriceSamples(testingTB *testing.T) {
	Convey("Given ticker observations", testingTB, func() {
		section := NewSection("BTC/EUR")
		start := time.Now()

		for index := range 20 {
			section.ObservePrice(
				"BTC/EUR",
				100+float64(index),
				start.Add(time.Duration(index)*ringSampleSpacing),
			)
		}

		Convey("It should retain enough samples for correlation", func() {
			So(len(section.PriceSamples("BTC/EUR")), ShouldBeGreaterThanOrEqualTo, minLagSamples)
		})
	})
}

func TestSectionCrossLagInsufficientData(testingTB *testing.T) {
	Convey("Given sparse histories", testingTB, func() {
		section := NewSection("BTC/EUR")
		now := time.Now()

		section.ObservePrice("BTC/EUR", 100, now)
		section.ObservePrice("ETH/EUR", 200, now)

		features := section.Features("ETH/EUR")

		Convey("It should refuse to score lag without enough samples", func() {
			So(features.LagOK, ShouldBeFalse)
		})
	})
}

func TestRecentPathMove(testingTB *testing.T) {
	Convey("Given a flat anchor path across the lag window", testingTB, func() {
		start := time.Now()
		samples := make([]correlation.Sample, minLagSamples)

		for index := range minLagSamples {
			samples[index] = correlation.Sample{
				At:    start.Add(time.Duration(index) * 2 * time.Minute),
				Value: 50000,
			}
		}

		move, ok := recentPathMove(samples, time.Duration(maxLagBars)*barInterval)

		Convey("It should report a near-zero move", func() {
			So(ok, ShouldBeTrue)
			So(move, ShouldBeLessThan, 1e-6)
		})
	})
}

func TestSignalMeasureTickAnchorColdStart(testingTB *testing.T) {
	Convey("Given an anchor before the move baseline warms", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

		seedTickerSeries(signal, "BTC/USD", start, 8, ringSampleSpacing, func(int) float64 {
			return 50000
		})

		result := signal.Measure(measurementQuery("BTC/USD"))

		Convey("It should withhold until the move baseline warms", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "BTC/USD"), ShouldBeFalse)
		})
	})
}

func TestSignalMeasureTickFollowerColdStart(testingTB *testing.T) {
	Convey("Given aligned anchor and follower paths before the move gate warms", testingTB, func() {
		viper.Set("market.anchor_symbol", "BTC/EUR")
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

		seedTickerSeries(signal, "BTC/EUR", start, minLagSamples, ringSampleSpacing, func(index int) float64 {
			return 50000 + float64(index)
		})
		seedTickerSeries(signal, "ETH/EUR", start, minLagSamples, ringSampleSpacing, func(index int) float64 {
			return 100 + float64(index)*2
		})

		insertLagFeatures(signal, "ETH/EUR", syncDriftPayload())
		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should publish synchronized drift from tree features", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ETH/EUR"), ShouldBeTrue)
			result.Release()
		})
	})
}
