package leadlag

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
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
