package learning

import (
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestGridStep(t *testing.T) {
	Convey("Given quantities with shared and opposing activation profiles", t, func() {
		grid := NewGrid()
		measurement := data.NewMeasurement[float64]("", "context", "source", time.Time{}, time.Time{})
		// Two independent alternating sequences span the requested plane.
		left := [...]float64{-1, 1, -1, 1, -1, 1, -1, 1}
		right := [...]float64{-1, -1, 1, 1, -1, -1, 1, 1}

		for index := range left {
			measurement.PutMetric(data.Metric[float64]{Label: "alpha", Raw: left[index]})
			measurement.PutMetric(data.Metric[float64]{Label: "beta", Raw: left[index]})
			measurement.PutMetric(data.Metric[float64]{Label: "opposite", Raw: -left[index]})
			measurement.PutMetric(data.Metric[float64]{Label: "independent", Raw: right[index]})
			So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
		}

		alpha := measurement.Metrics["alpha"].Coordinates
		beta := measurement.Metrics["beta"].Coordinates
		opposite := measurement.Metrics["opposite"].Coordinates
		independent := measurement.Metrics["independent"].Coordinates
		So(alpha, ShouldNotBeNil)
		So(beta, ShouldNotBeNil)
		So(opposite, ShouldNotBeNil)
		alphaColumn := grid.columnIndex[[2]string{"source", "alpha"}]
		betaColumn := grid.columnIndex[[2]string{"source", "beta"}]
		oppositeColumn := grid.columnIndex[[2]string{"source", "opposite"}]

		for dimension := range gridDimensions {
			So(grid.basis[dimension][betaColumn], ShouldAlmostEqual, grid.basis[dimension][alphaColumn])
			So(grid.basis[dimension][oppositeColumn], ShouldAlmostEqual, -grid.basis[dimension][alphaColumn])
		}

		So(math.Hypot(alpha[0]-independent[0], alpha[1]-independent[1]), ShouldBeGreaterThan, 0)
		So(grid.Version, ShouldEqual, len(left))

		Convey("the existing coordinates and storage survive another update", func() {
			values := &grid.Values[0][0]
			measurement.PutMetric(data.Metric[float64]{Label: "alpha", Raw: 2})
			So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			So(measurement.Metrics["alpha"].Coordinates, ShouldEqual, alpha)
			So(&grid.Values[0][0], ShouldEqual, values)
			column := grid.columnIndex[[2]string{"source", "alpha"}]
			So(grid.Values[0][column], ShouldEqual, 2)
		})

		Convey("missing values become absent while a measured zero remains present", func() {
			delete(measurement.Metrics, "alpha")
			measurement.PutMetric(data.Metric[float64]{Label: "beta", Raw: 0})
			So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			So(grid.Present[0][grid.columnIndex[[2]string{"source", "alpha"}]], ShouldBeFalse)
			So(grid.Present[0][grid.columnIndex[[2]string{"source", "beta"}]], ShouldBeTrue)
			So(grid.Coordinates[grid.columnIndex[[2]string{"source", "alpha"}]], ShouldEqual, alpha)
			So(grid.activations[0][grid.columnIndex[[2]string{"source", "alpha"}]], ShouldEqual, 0)
		})

		Convey("a relationship that reverses inconsistently separates", func() {
			// The first half was a persistent inverse. It now becomes a copy,
			// while beta continues to match throughout the whole sequence.
			for _, value := range left {
				measurement.PutMetric(data.Metric[float64]{Label: "alpha", Raw: value})
				measurement.PutMetric(data.Metric[float64]{Label: "beta", Raw: value})
				measurement.PutMetric(data.Metric[float64]{Label: "opposite", Raw: value})
				So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			}

			So(math.Hypot(
				grid.basis[0][alphaColumn]-grid.basis[0][betaColumn],
				grid.basis[1][alphaColumn]-grid.basis[1][betaColumn]), ShouldAlmostEqual, 0)
			So(math.Hypot(
				grid.basis[0][alphaColumn]-grid.basis[0][oppositeColumn],
				grid.basis[1][alphaColumn]-grid.basis[1][oppositeColumn]), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given different numerical units and independent context histories", t, func() {
		grid := NewGrid()
		first := data.NewMeasurement[float64]("", "first", "source", time.Time{}, time.Time{})
		second := data.NewMeasurement[float64]("", "second", "source", time.Time{}, time.Time{})

		for _, value := range []float64{-1, 1, -1, 1, -1, 1} {
			first.PutMetric(data.Metric[float64]{Label: "alpha", Raw: value})
			first.PutMetric(data.Metric[float64]{Label: "scaled", Raw: 1000 + value*100})
			So(grid.Step([]*data.Measurement[float64]{first}), ShouldBeNil)
			second.PutMetric(data.Metric[float64]{Label: "alpha", Raw: 100000})
			So(grid.Step([]*data.Measurement[float64]{second}), ShouldBeNil)
		}

		alpha := first.Metrics["alpha"].Coordinates
		alphaColumn := grid.columnIndex[[2]string{"source", "alpha"}]
		scaledColumn := grid.columnIndex[[2]string{"source", "scaled"}]

		for dimension := range gridDimensions {
			So(grid.basis[dimension][scaledColumn], ShouldAlmostEqual, grid.basis[dimension][alphaColumn])
		}

		So(second.Metrics["alpha"].Coordinates, ShouldEqual, alpha)
		So(grid.Values[grid.rowIndex["first"]][grid.columnIndex[[2]string{"source", "alpha"}]], ShouldEqual, 1)
		So(grid.Values[grid.rowIndex["second"]][grid.columnIndex[[2]string{"source", "alpha"}]], ShouldEqual, 100000)
	})

	Convey("Given the same observations with different producer evidence", t, func() {
		grid := NewGrid()
		strong := data.NewMeasurement[float64]("", "context", "strong", time.Time{}, time.Time{})
		weak := data.NewMeasurement[float64]("", "context", "weak", time.Time{}, time.Time{})
		unknown := data.NewMeasurement[float64]("", "context", "unknown", time.Time{}, time.Time{})
		strong.Metadata = map[string]float64{data.MetadataSupport: 10, data.MetadataMahalanobisSNR: 9}
		weak.Metadata = map[string]float64{data.MetadataSupport: 2, data.MetadataMahalanobisSNR: 1}
		unknown.Metadata = map[string]float64{data.MetadataSupport: 10}

		for _, value := range []float64{-1, 1, -1, 1} {
			for _, measurement := range []*data.Measurement[float64]{strong, weak, unknown} {
				measurement.PutMetric(data.Metric[float64]{Label: "value", Raw: value})
			}

			So(grid.Step([]*data.Measurement[float64]{strong, weak, unknown}), ShouldBeNil)
		}

		strongPoint := strong.Metrics["value"].Coordinates
		weakPoint := weak.Metrics["value"].Coordinates
		So(math.Hypot(strongPoint[0]-weakPoint[0], strongPoint[1]-weakPoint[1]), ShouldBeGreaterThan, 0)
		So(grid.weights[grid.columnIndex[[2]string{"strong", "value"}]], ShouldBeGreaterThan,
			grid.weights[grid.columnIndex[[2]string{"weak", "value"}]])
		So(grid.Present[0][grid.columnIndex[[2]string{"unknown", "value"}]], ShouldBeTrue)
		So(grid.activations[0][grid.columnIndex[[2]string{"unknown", "value"}]], ShouldNotEqual, 0)
		So(unknown.SNRDefined, ShouldBeFalse)

		Convey("withdrawn producer noise evidence uses only the grid's local change history", func() {
			delete(strong.Metadata, data.MetadataMahalanobisSNR)
			strong.PutMetric(data.Metric[float64]{Label: "value", Raw: -1})
			So(grid.Step([]*data.Measurement[float64]{strong}), ShouldBeNil)
			So(strong.SNRDefined, ShouldBeFalse)
			So(grid.activations[0][grid.columnIndex[[2]string{"strong", "value"}]], ShouldNotEqual, 0)
			So(grid.activations[0][grid.columnIndex[[2]string{"weak", "value"}]], ShouldEqual, 0)
		})
	})

	Convey("Given values that all change on every update", t, func() {
		grid := NewGrid()
		measurement := data.NewMeasurement[float64]("", "context", "source", time.Time{}, time.Time{})
		left := [...]float64{-1, 1, -1, 1, -1, 1, -1, 1}
		right := [...]float64{-1, 0, 1, 0, -1, 0, 1, 0}

		for index, value := range left {
			measurement.PutMetric(data.Metric[float64]{Label: "alpha", Raw: value})
			measurement.PutMetric(data.Metric[float64]{Label: "inverse", Raw: -value})
			measurement.PutMetric(data.Metric[float64]{Label: "independent", Raw: right[index]})
			So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
		}

		alpha := measurement.Metrics["alpha"].Coordinates
		independent := measurement.Metrics["independent"].Coordinates
		alphaColumn := grid.columnIndex[[2]string{"source", "alpha"}]
		inverseColumn := grid.columnIndex[[2]string{"source", "inverse"}]

		for dimension := range gridDimensions {
			So(grid.basis[dimension][inverseColumn], ShouldAlmostEqual, -grid.basis[dimension][alphaColumn])
		}

		So(math.Hypot(alpha[0]-independent[0], alpha[1]-independent[1]), ShouldBeGreaterThan, 0)

		Convey("an unchanged value has no movement even while away from its baseline", func() {
			So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)

			for _, activation := range grid.activations[0] {
				So(activation, ShouldEqual, 0)
			}
		})
	})

	Convey("Given a rejected or mixed-context update", t, func() {
		grid := NewGrid()
		failure := errors.New("unavailable input")
		measurement := data.NewMeasurement[float64]("", "first", "source", time.Time{}, time.Time{})
		measurement.PutMetric(data.Metric[float64]{Label: "value", Raw: 1})
		So(grid.Step([]*data.Measurement[float64]{measurement, {Err: failure}}), ShouldEqual, failure)
		So(grid.Rows, ShouldBeEmpty)
		other := data.NewMeasurement[float64]("", "second", "source", time.Time{}, time.Time{})
		So(grid.Step([]*data.Measurement[float64]{measurement, other}), ShouldNotBeNil)
		So(grid.Rows, ShouldBeEmpty)
		So(grid.Step(nil), ShouldBeNil)
		So(grid.Version, ShouldEqual, 0)
	})
}

func BenchmarkGridStep(b *testing.B) {
	// The source catalog spans hundreds of quantities; contexts reuse its
	// column layout while retaining their own values and adaptive baselines.
	for _, columns := range []int{32, 404} {
		b.Run(strconv.Itoa(columns), func(b *testing.B) {
			grid := NewGrid()
			measurement := data.NewMeasurement[float64]("", "context", "source", time.Time{}, time.Time{})

			for column := range columns {
				label := strconv.Itoa(column)
				measurement.PutMetric(data.Metric[float64]{Label: label, Raw: float64(column)})
			}

			for range 4 {
				if err := grid.Step([]*data.Measurement[float64]{measurement}); err != nil {
					b.Fatal(err)
				}
			}

			b.ReportAllocs()

			for b.Loop() {
				for key, metric := range measurement.Metrics {
					metric.Raw = -metric.Raw
					measurement.Metrics[key] = metric
				}

				if err := grid.Step([]*data.Measurement[float64]{measurement}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
