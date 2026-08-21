package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var benchmarkMeasurement any

func TestSignalNewMeasurement(t *testing.T) {
	Convey("Given an observed PumpDump interval", t, func() {
		at := time.Unix(1_786_099_210, 250).UTC()
		from := at.Add(-5 * time.Second)
		signal := &Signal{}
		measurement := signal.newMeasurement(at, from, 0.75, 0.6)

		Convey("It preserves complete provenance and estimator confidence", func() {
			So(measurement.Source, ShouldEqual, string(types.SourcePumpDump))
			So(measurement.At, ShouldResemble, at)
			So(measurement.ObservedFrom, ShouldResemble, from)
			So(measurement.Horizon, ShouldEqual, 5*time.Second)
			So(measurement.Maturity, ShouldEqual, 0.75)
			separation := measurement.Metrics[string(
				types.MetricHypothesisSeparation,
			)]
			So(separation.Raw, ShouldEqual, 0.6)
			So(separation.Normalized, ShouldNotBeNil)
			So(*separation.Normalized, ShouldEqual, 0.6)
		})
	})
}

func TestSignalBookMeasurement(t *testing.T) {
	Convey("Given an executable interval wider than its center", t, func() {
		at := time.Unix(1_786_099_210, 250).UTC()
		geometry := bookGeometry()
		measurement := (&Signal{separate: statistic.Separation}).bookMeasurement(
			at,
			geometry,
			nomagique.Frame{},
			nomagique.Frame{},
		)

		Convey("It preserves raw relative spread and publishes bounded evidence", func() {
			spread := measurement.Metrics[string(types.MetricSpread)]
			So(spread.Raw, ShouldEqual, 3.0)
			So(spread.Normalized, ShouldNotBeNil)
			So(*spread.Normalized, ShouldEqual, 0.6)
			So(measurement.Metadata["relative_spread"], ShouldEqual, 1.2)
		})
	})

	Convey("Given a causal sequence of executable intervals", t, func() {
		at := time.Unix(1_786_099_210, 250).UTC()
		geometry := nomagique.NewStream(equation.Geometry(), nomagique.Frame{})
		signal := &Signal{separate: statistic.Separation}
		first, err := geometry.Step(bookGeometryInput(at, 99, 101))
		So(err, ShouldBeNil)
		firstMeasurement := signal.bookMeasurement(
			at,
			first,
			nomagique.Frame{},
			nomagique.Frame{},
		)

		Convey("It withholds compression separation before a baseline exists", func() {
			_, hasCompression := firstMeasurement.Metrics[string(
				types.MetricCompression,
			)]
			So(hasCompression, ShouldBeFalse)
			So(firstMeasurement.Maturity, ShouldEqual, 0.5)
			separation := firstMeasurement.Metrics[string(
				types.MetricHypothesisSeparation,
			)]
			So(separation.Raw, ShouldEqual, 0.0)
			So(*separation.Normalized, ShouldEqual, 0.0)
		})

		secondAt := at.Add(time.Second)
		second, err := geometry.Step(bookGeometryInput(secondAt, 99, 101))
		So(err, ShouldBeNil)
		secondMeasurement := signal.bookMeasurement(
			secondAt,
			second,
			nomagique.Frame{},
			nomagique.Frame{},
		)

		Convey("It reports no distinction from an equal retained baseline", func() {
			compression := secondMeasurement.Metrics[string(types.MetricCompression)]
			So(compression.Normalized, ShouldNotBeNil)
			So(*compression.Normalized, ShouldEqual, 0.0)
			separation := secondMeasurement.Metrics[string(
				types.MetricHypothesisSeparation,
			)]
			So(*separation.Normalized, ShouldEqual, 0.0)
			So(secondMeasurement.Maturity, ShouldEqual, 2.0/3.0)
		})

		thirdAt := secondAt.Add(time.Second)
		third, err := geometry.Step(bookGeometryInput(thirdAt, 99.5, 100.5))
		So(err, ShouldBeNil)
		thirdMeasurement := signal.bookMeasurement(
			thirdAt,
			third,
			nomagique.Frame{},
			nomagique.Frame{},
		)

		Convey("It publishes the contraction and its earned margin together", func() {
			compression := thirdMeasurement.Metrics[string(types.MetricCompression)]
			So(compression.Normalized, ShouldNotBeNil)
			So(*compression.Normalized, ShouldEqual, 0.5)
			separation := thirdMeasurement.Metrics[string(
				types.MetricHypothesisSeparation,
			)]
			So(separation.Raw, ShouldEqual, 0.5)
			So(*separation.Normalized, ShouldEqual, 0.5)
			So(thirdMeasurement.Maturity, ShouldEqual, 2.0/3.0)
		})
	})
}

func bookGeometryInput(
	at time.Time,
	alphaPrice float64,
	betaPrice float64,
) nomagique.Frame {
	input := eventFrame(at)
	input.Put(nmtypes.AlphaPrice, alphaPrice)
	input.Put(nmtypes.BetaPrice, betaPrice)
	input.Put(nmtypes.AlphaQuantity, 10)
	input.Put(nmtypes.BetaQuantity, 10)

	return input
}

func bookGeometry() nomagique.Frame {
	geometry := nomagique.Frame{}
	geometry.Put(nmtypes.AlphaPrice, 1)
	geometry.Put(nmtypes.BetaPrice, 4)
	geometry.Put(nmtypes.AlphaQuantity, 10)
	geometry.Put(nmtypes.BetaQuantity, 20)
	geometry.Put(equation.SymbolCenter, 2.5)
	geometry.Put(equation.SymbolWidth, 3)
	geometry.Put(equation.SymbolRelativeWidth, 1.2)
	geometry.Put(equation.SymbolDissimilarity, 0.6)
	geometry.Put(equation.SymbolBalance, 0)
	geometry.Put(statistic.SymbolMaturity, 0.5)

	return geometry
}

func BenchmarkSignalNewMeasurement(b *testing.B) {
	at := time.Unix(1_786_099_210, 250).UTC()
	from := at.Add(-5 * time.Second)
	signal := &Signal{}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		benchmarkMeasurement = signal.newMeasurement(at, from, 0.75, 0.6)
	}
}

func BenchmarkSignalBookMeasurement(b *testing.B) {
	at := time.Unix(1_786_099_210, 250).UTC()
	signal := &Signal{separate: statistic.Separation}
	geometryStream := nomagique.NewStream(equation.Geometry(), nomagique.Frame{})
	_, err := geometryStream.Step(bookGeometryInput(at, 99, 101))

	if err != nil {
		b.Fatal(err)
	}

	_, err = geometryStream.Step(bookGeometryInput(at.Add(time.Second), 99, 101))

	if err != nil {
		b.Fatal(err)
	}

	geometry, err := geometryStream.Step(bookGeometryInput(
		at.Add(2*time.Second),
		99.5,
		100.5,
	))

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		benchmarkMeasurement = signal.bookMeasurement(
			at,
			geometry,
			nomagique.Frame{},
			nomagique.Frame{},
		)
	}
}
