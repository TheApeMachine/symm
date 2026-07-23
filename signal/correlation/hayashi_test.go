package correlation

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"
)

/*
TestOverlapCovarianceRecentMatchesForward proves the backward tail scan equals
a full forward overlap sum on a dense peer path.
*/
func TestOverlapCovarianceRecentMatchesForward(t *testing.T) {
	Convey("Given a long right path and a newest left interval", t, func() {
		start := time.Unix(1_700_000_000, 0).UTC()
		right := make([]nomcorrelation.Sample, 0, 64)
		rightLogs := make([]float64, 0, 64)

		for index := range 64 {
			price := 100 + float64(index)*0.1
			right = append(right, nomcorrelation.Sample{
				At:    start.Add(time.Duration(index) * time.Second),
				Value: price,
			})
			rightLogs = append(rightLogs, math.Log(price))
		}

		leftStart := right[len(right)-3].At
		leftEnd := right[len(right)-1].At.Add(500 * time.Millisecond)
		leftReturn := 0.01
		recent := overlapCovarianceRecent(
			leftReturn, leftStart, leftEnd, right, rightLogs,
		)
		retired := overlapCovarianceRetired(
			leftReturn, leftStart, leftEnd, right, rightLogs,
		)

		Convey("It should match the forward retired scan on the same window", func() {
			So(recent, ShouldAlmostEqual, retired, 1e-12)
		})
	})
}

/*
TestIncrementalPairTracksHayashi proves streaming covariance stays aligned with
the full Hayashi estimator after appends and left-edge trims.
*/
func TestIncrementalPairTracksHayashi(t *testing.T) {
	Convey("Given two symbols fed asynchronously", t, func() {
		section := NewSection()
		start := time.Unix(1_700_000_000, 0).UTC()

		for index := range 40 {
			at := start.Add(time.Duration(index) * time.Second)
			_, err := section.Measure([]kraken.TickerData{
				{
					Symbol:    "A/USD",
					Timestamp: at,
					Last:      decimal.NewFromFloat64(100 + float64(index)*0.2),
				},
				{
					Symbol:    "B/USD",
					Timestamp: at.Add(200 * time.Millisecond),
					Last:      decimal.NewFromFloat64(50 + float64(index)*0.1),
				},
			})
			So(err, ShouldBeNil)
		}

		left := section.symbols["A/USD"]
		right := section.symbols["B/USD"]
		pair := section.pair("A/USD", "B/USD")
		So(left, ShouldNotBeNil)
		So(right, ShouldNotBeNil)
		So(pair, ShouldNotBeNil)

		expected, ok := algorithm.HayashiPairCorrelation(left.samples, right.samples, 0)
		So(ok, ShouldBeTrue)
		got, gotOK := pairCorrelation(pair, left.variance, right.variance)
		So(gotOK, ShouldBeTrue)

		Convey("It should match the full pairwise Hayashi correlation", func() {
			So(got, ShouldAlmostEqual, expected, 1e-9)
		})
	})
}

/*
TestDropLeftEdgeClearsVariance proves a depleted path cannot retain a stale
Hayashi variance denominator.
*/
func TestDropLeftEdgeClearsVariance(t *testing.T) {
	Convey("Given a symbol reduced below two samples", t, func() {
		section := NewSection()
		state := section.ensure("A/USD")
		state.samples = []nomcorrelation.Sample{{
			At: time.Unix(1, 0), Value: 1,
		}}
		state.logPrices = []float64{0}
		state.variance = 1.25
		section.dropLeftEdge("A/USD", state)

		Convey("It should zero variance with the emptied path", func() {
			So(state.variance, ShouldEqual, 0)
			So(state.samples, ShouldBeEmpty)
			So(state.logPrices, ShouldBeEmpty)
		})
	})
}

/*
BenchmarkOverlapCovarianceRecent measures the backward tail scan against a
dense peer history.
*/
func BenchmarkOverlapCovarianceRecent(benchmark *testing.B) {
	start := time.Unix(1_700_000_000, 0).UTC()
	right := make([]nomcorrelation.Sample, 512)
	rightLogs := make([]float64, 512)

	for index := range right {
		price := 100 + float64(index)*0.01
		right[index] = nomcorrelation.Sample{
			At:    start.Add(time.Duration(index) * time.Millisecond),
			Value: price,
		}
		rightLogs[index] = math.Log(price)
	}

	leftStart := right[len(right)-2].At
	leftEnd := right[len(right)-1].At.Add(time.Millisecond)

	benchmark.ResetTimer()

	for benchmark.Loop() {
		_ = overlapCovarianceRecent(
			0.01, leftStart, leftEnd, right, rightLogs,
		)
	}
}
