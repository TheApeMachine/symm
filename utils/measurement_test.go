package utils_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
TestLatestMeasurements proves epoch selection happens within the requested
source and retains every requested metric for each symbol in that epoch.
*/
func TestLatestMeasurements(t *testing.T) {
	observedAt := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	measurements := []*types.Measurement{
		{Source: types.SourcePumpDump, Metric: types.MetricStrength, Symbol: "SIM1/USD", At: observedAt, Raw: 1},
		{Source: types.SourcePumpDump, Metric: types.MetricStrength, Symbol: "SIM1/USD", At: observedAt.Add(time.Second), Raw: 2},
		{Source: types.SourceCVD, Metric: types.MetricStrength, Symbol: "SIM1/USD", At: observedAt.Add(2 * time.Second), Raw: 3},
	}

	Convey("Given measurements from overlapping signal epochs", t, func() {
		values := utils.LatestMeasurements(
			measurements,
			types.SourcePumpDump,
			[]types.MetricType{types.MetricStrength},
		)

		Convey("It should select the requested source's latest value", func() {
			So(values[types.MetricStrength]["SIM1/USD"], ShouldEqual, 2)
		})
	})
}

/*
TestPeakMeasurements proves peak selection handles negative values and does not
admit measurements from unrequested sources or metrics.
*/
func TestPeakMeasurements(t *testing.T) {
	measurements := []*types.Measurement{
		{Source: types.SourceCVD, Metric: types.MetricNet, Symbol: "SIM1/USD", Raw: -3},
		{Source: types.SourceCVD, Metric: types.MetricNet, Symbol: "SIM1/USD", Raw: -1},
		{Source: types.SourcePumpDump, Metric: types.MetricNet, Symbol: "SIM1/USD", Raw: 4},
	}

	Convey("Given signed measurements from multiple sources", t, func() {
		values := utils.PeakMeasurements(
			measurements,
			types.SourceCVD,
			[]types.MetricType{types.MetricNet, types.MetricStrength},
		)

		Convey("It should retain the largest requested value", func() {
			So(values[types.MetricNet]["SIM1/USD"], ShouldEqual, -1)
			So(values[types.MetricStrength], ShouldBeEmpty)
		})
	})
}

/*
TestPeakMagnitudeMeasurements proves signed evidence is selected by excursion
size without losing the sign that gives the measurement its meaning.
*/
func TestPeakMagnitudeMeasurements(t *testing.T) {
	measurements := []*types.Measurement{
		{Source: types.SourceFluid, Metric: types.MetricSourceBalance, Symbol: "SIM1/USD", Raw: 0},
		{Source: types.SourceFluid, Metric: types.MetricSourceBalance, Symbol: "SIM1/USD", Raw: -3},
		{Source: types.SourceFluid, Metric: types.MetricSourceBalance, Symbol: "SIM1/USD", Raw: 2},
		{Source: types.SourceCVD, Metric: types.MetricSourceBalance, Symbol: "SIM1/USD", Raw: 4},
	}

	Convey("Given signed measurements with positive and negative excursions", t, func() {
		values := utils.PeakMagnitudeMeasurements(
			measurements,
			types.SourceFluid,
			[]types.MetricType{types.MetricSourceBalance},
		)

		Convey("It should retain the largest requested magnitude and its sign", func() {
			So(values[types.MetricSourceBalance]["SIM1/USD"], ShouldEqual, -3)
		})
	})
}

/*
BenchmarkPeakMeasurements measures the reducer against a realistic multi-symbol
signal batch used by the market-facing tests.
*/
func BenchmarkPeakMeasurements(b *testing.B) {
	measurements := make([]*types.Measurement, 0, 3*32)

	for observation := range 32 {
		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			measurements = append(measurements, &types.Measurement{
				Source: types.SourcePumpDump,
				Metric: types.MetricStrength,
				Symbol: symbol,
				Raw:    float64(observation),
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		utils.PeakMeasurements(
			measurements,
			types.SourcePumpDump,
			[]types.MetricType{types.MetricStrength},
		)
	}
}

/*
BenchmarkPeakMagnitudeMeasurements measures signed-excursion reduction over a
realistic multi-symbol signal batch.
*/
func BenchmarkPeakMagnitudeMeasurements(b *testing.B) {
	measurements := make([]*types.Measurement, 0, 3*32)

	for observation := range 32 {
		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			measurements = append(measurements, &types.Measurement{
				Source: types.SourceFluid,
				Metric: types.MetricSourceBalance,
				Symbol: symbol,
				Raw:    float64(observation - 16),
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		utils.PeakMagnitudeMeasurements(
			measurements,
			types.SourceFluid,
			[]types.MetricType{types.MetricSourceBalance},
		)
	}
}
