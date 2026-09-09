package impulse

import (
	"errors"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/tests/market"
	"math"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestSolverStep(t *testing.T) {
	Convey("Signal and logic observations cross the grid stage with their own symbol identities", t, func() {
		solver := NewSolver()
		at := time.Unix(100, 0)
		measurement := data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", at, at.Add(-time.Second))
		measurement.PutMetric(data.Metric[float64]{Label: "delta", Raw: 3})
		envelope := &types.Envelope{CVD: measurement,
			Categories: []types.Category{{Symbol: "ETH/USD", At: at, Type: types.VerticalIgnition, Strength: 4, Maturity: .75}},
		}
		So(solver.Step(envelope), ShouldEqual, envelope)
		So(solver.Error(), ShouldBeNil)
		So(len(envelope.Impulses), ShouldEqual, 2)
		So(len(solver.Rows), ShouldEqual, 2)
		So(len(solver.Columns), ShouldEqual, 6)
		So(solver.Latest["BTC/USD"].From, ShouldEqual, measurement.From)
		So(solver.Latest["BTC/USD"].Ready, ShouldBeFalse)

		Convey("A failed producer remains an explicit stage error", func() {
			measurement.Err = errors.New("producer failed")
			solver.Step(envelope)
			So(solver.Error(), ShouldNotBeNil)
		})
	})
}

func BenchmarkSolverStep(b *testing.B) {
	solver := NewSolver()
	at := time.Unix(100, 0)
	measurement := data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", at, at)
	measurement.Metadata = map[string]float64{data.MetadataSupport: 100, data.MetadataMahalanobisSNR: 10}
	for index := range 412 {
		measurement.PutMetric(data.Metric[float64]{Label: strconv.Itoa(index), Raw: float64(index % 7)})
	}
	envelope := &types.Envelope{CVD: measurement}
	b.ReportAllocs()
	for b.Loop() {
		measurement.At = measurement.At.Add(time.Millisecond)
		for label, metric := range measurement.Metrics {
			metric.Raw = -metric.Raw
			measurement.Metrics[label] = metric
		}
		solver.Step(envelope)
		if err := solver.Error(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSolverStepCategoryStartup(t *testing.T) {
	Convey("Sparse category evidence remains usable through live and persisted grid inputs", t, func() {
		classifier := category.NewSolver(t.Context())
		defer func() { So(classifier.Close(), ShouldBeNil) }()
		live, replay := NewSolver(), grid.NewSpace()
		tape := market.NewOpportunityTape("CCD/USD", time.Unix(100, 0), 6)
		for index, step := range tape.Steps {
			// The fixture's alternating bipolar context maps to zero/one
			// affinities, exercising support appearing and disappearing.
			affinity := (step.Context + 1) / 2
			measurement := data.NewMeasurement[float64]("startup", tape.Symbol, "cvd", step.EventTime, tape.Steps[0].EventTime)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "signed_net_fraction_zscore", Raw: step.Context, Normalized: &affinity})
			envelope := &types.Envelope{Key: tape.Symbol, CVD: measurement,
				CaptureID: hindsight.CaptureIdentity{Run: "category-startup", Sequence: hindsight.CaptureSequence(index + 1)},
			}
			So(classifier.Step(envelope), ShouldEqual, envelope)
			So(classifier.Error(), ShouldBeNil)
			if affinity == 0 {
				So(envelope.Categories, ShouldBeEmpty)
			}

			if affinity > 0 {
				So(envelope.Categories, ShouldNotBeEmpty)
			}
			unsupported := 0
			for _, reading := range envelope.Categories {
				So(reading.Confidence, ShouldBeGreaterThan, 0)
				So(reading.Surprisal, ShouldAlmostEqual, -math.Log2(reading.Confidence))

				if reading.Strength == 0 {
					unsupported++
				}
			}
			if affinity > 0 {
				So(unsupported, ShouldBeGreaterThan, 0)
			}
			captured, err := (hindsight.ArtifactWitness{
				Envelope: hindsight.EnvelopeRef{Origin: envelope.CaptureID}, Payload: envelope.EncodePrecursor(),
			}).RehearsalInput()
			So(err, ShouldBeNil)
			So(live.Step(envelope), ShouldEqual, envelope)
			So(live.Error(), ShouldBeNil)
			So(replay.Step(captured.Measurements), ShouldBeNil)
			So(live.Version, ShouldEqual, index+1)
			So(replay.Version, ShouldEqual, live.Version)
		}
	})
}
