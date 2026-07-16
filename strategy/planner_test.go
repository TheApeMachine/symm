package strategy

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

type plannerCutSignal struct {
	high    int
	cut     int
	seen    int
	measure func()
}

/*
plannerWorkSignal supplies repeatable numerical work for Planner.Update
benchmarks without coupling the strategy package to concrete signal internals.
*/
type plannerWorkSignal struct {
	values []float64
}

/*
plannerConcurrentSignal blocks after announcing its start so tests can prove
that Planner launches every signal before collecting results.
*/
type plannerConcurrentSignal struct {
	source  types.SourceType
	started chan<- types.SourceType
	release <-chan struct{}
}

/*
Measure publishes isolated evidence around the synchronization point used by
the concurrency test.
*/
func (signal *plannerConcurrentSignal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	signal.started <- signal.source
	<-signal.release
	thesis.Measurements = append(thesis.Measurements, &types.Measurement{
		Source: signal.source,
		Metric: types.MetricStrength,
		At:     thesis.At,
	})
	thesis.CrossSection.Metrics = append(
		thesis.CrossSection.Metrics,
		types.SymbolMetric{Symbol: string(signal.source), At: thesis.At},
	)
	thesis.Signals.Store(string(signal.source), signal.source)

	return thesis
}

/*
Measure reduces the benchmark fixture and retains its result so the compiler
cannot eliminate the work.
*/
func (signal *plannerWorkSignal) Measure(thesis *types.Thesis) *types.Thesis {
	total := 0.0

	for _, value := range signal.values {
		total += math.Sqrt(value)
	}

	thesis.Measurements = append(thesis.Measurements, &types.Measurement{
		Source: types.SourceCorrelation,
		Metric: types.MetricStrength,
		Raw:    total,
		At:     thesis.At,
	})

	return thesis
}

func (signal *plannerCutSignal) Capture(time.Time) error {
	signal.cut = signal.high
	return nil
}

func (signal *plannerCutSignal) Measure(thesis *types.Thesis) *types.Thesis {
	signal.seen = signal.cut

	if signal.measure != nil {
		signal.measure()
	}

	return thesis
}

func TestPlanner_UpdateCapturesInputsFirst(t *testing.T) {
	Convey("Given an observation arriving while an earlier signal measures", t, func() {
		later := &plannerCutSignal{}
		earlier := &plannerCutSignal{
			measure: func() {
				later.high++
			},
		}
		planner := NewPlanner(
			context.Background(),
			nil,
			[]types.Signal{earlier, later},
			nil,
		)
		t.Cleanup(planner.Close)

		planner.Update("", "")

		Convey("It defers that observation until the next Thesis", func() {
			So(later.seen, ShouldEqual, 0)
			planner.Update("", "")
			So(later.seen, ShouldEqual, 1)
		})
	})
}

func TestPlanner_UpdateMeasuresSignalsConcurrently(t *testing.T) {
	Convey("Given two independent signals blocked inside measurement", t, func() {
		started := make(chan types.SourceType, 2)
		release := make(chan struct{})
		completed := make(chan *types.Thesis, 1)
		signals := []types.Signal{
			&plannerConcurrentSignal{
				source: types.SourceCorrelation, started: started, release: release,
			},
			&plannerConcurrentSignal{
				source: types.SourceCVD, started: started, release: release,
			},
		}
		planner := NewPlanner(context.Background(), nil, signals, nil)
		t.Cleanup(planner.Close)

		go func() {
			completed <- planner.Update("", "")
		}()

		first := <-started
		var second types.SourceType

		select {
		case second = <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("second signal did not start concurrently")
		}

		close(release)
		thesis := <-completed

		Convey("It should merge isolated results in configured order", func() {
			So(first, ShouldNotEqual, second)
			So(thesis.Measurements, ShouldHaveLength, 2)
			So(thesis.Measurements[0].Source, ShouldEqual, types.SourceCorrelation)
			So(thesis.Measurements[1].Source, ShouldEqual, types.SourceCVD)
			So(thesis.CrossSection.Metrics, ShouldHaveLength, 2)
			_, correlationFound := thesis.Signals.Load(string(types.SourceCorrelation))
			_, cvdFound := thesis.Signals.Load(string(types.SourceCVD))
			So(correlationFound, ShouldBeTrue)
			So(cvdFound, ShouldBeTrue)
		})
	})
}

func TestPlanner_CloseReleasesMeasurementWait(t *testing.T) {
	Convey("Given a worker still measuring when Planner closes", t, func() {
		started := make(chan types.SourceType, 1)
		release := make(chan struct{})
		completed := make(chan *types.Thesis, 1)
		planner := NewPlanner(
			context.Background(),
			nil,
			[]types.Signal{&plannerConcurrentSignal{
				source: types.SourceCorrelation, started: started, release: release,
			}},
			nil,
		)

		go func() {
			completed <- planner.Update("", "")
		}()

		<-started
		planner.Close()

		select {
		case <-completed:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("Planner remained blocked after close")
		}

		close(release)
	})
}

func TestPlannerDecide(t *testing.T) {
	forecast := types.Forecasts{
		Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		ObservedInterval: time.Second, SourceEpoch: 1, HorizonEvents: 1,
		ExpiresEpoch: 2, Target: "mid_log_return", ModelVersion: "online",
		Ready: true, Calibrated: true, FrictionReady: true,
		CalibrationSamples: 1, ExpectedReturn: 0.05, ReferencePrice: 100,
		BuyCapacity: 100, SellCapacity: 100, ExpectedSpread: 0.001,
		ExpectedImpact: 0.001, ExpectedAdverseSelection: 0.001,
		Confidence: 1,
	}
	planner := &Planner{}

	Convey("Given an eligible forecast without cognitive memory", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, forecast)

		planner.Decide(thesis, map[string]float64{"BTC/USD": 0.001}, 100, 1)

		Convey("It should record no action without creating a broker order", func() {
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "nothing")
			So(thesis.Decisions[0].Cause, ShouldEqual, "cognitive_not_ready")
			So(thesis.Orders, ShouldBeEmpty)
			So(thesis.Positions, ShouldHaveLength, 1)
			So(thesis.Positions[0].Symbol, ShouldEqual, "BTC/USD")
			So(thesis.Positions[0].Qty.Sign(), ShouldEqual, 0)
		})
	})

	Convey("Given ready DMT memory that supports a buy entry", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, forecast)
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Source: "dmt", Symbol: "BTC/USD", At: forecast.At,
			Ready: true, Winner: "buy",
		})

		planner.Decide(thesis, map[string]float64{"BTC/USD": 0.001}, 100, 1)

		Convey("It should retain the decision and emit the selected entry", func() {
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "enter")
			So(thesis.Orders, ShouldHaveLength, 1)
			So(thesis.Orders[0].Description.Type, ShouldEqual, "enter")
			So(thesis.Orders[0].Volume.Float64(), ShouldAlmostEqual,
				thesis.Decisions[0].ProposedQuantity, 1e-12)
			So(thesis.Orders[0].Volume.Float64(), ShouldBeLessThan,
				thesis.Decisions[0].ProposedNotional)
			So(thesis.Positions, ShouldHaveLength, 1)
			So(thesis.Positions[0].Qty.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given an open position without ready cognitive memory", t, func() {
		thesis := types.NewThesis(nil)
		exitForecast := forecast
		exitForecast.ExpectedReturn = -0.05
		thesis.Forecasts = append(thesis.Forecasts, exitForecast)
		thesis.Positions = append(thesis.Positions, types.Holding{
			Order: &spot.Order{Description: &spot.OrderDescription{Pair: "BTC/USD"}},
			Qty:   decimal.NewFromFloat64(0.5),
			Mark:  decimal.NewFromFloat64(100),
		})

		planner.Decide(thesis, map[string]float64{"BTC/USD": 0.001}, 100, 1)

		Convey("It should still manage and exit the existing exposure", func() {
			So(thesis.Decisions, ShouldHaveLength, 1)
			So(thesis.Decisions[0].Action, ShouldEqual, "exit")
			So(thesis.Orders, ShouldHaveLength, 1)
			So(thesis.Orders[0].Description.Type, ShouldEqual, "exit")
		})
	})
}

func BenchmarkPlannerDecide(b *testing.B) {
	planner := &Planner{}
	forecast := types.Forecasts{
		Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		ObservedInterval: time.Second, SourceEpoch: 1, HorizonEvents: 1,
		ExpiresEpoch: 2, Target: "mid_log_return", ModelVersion: "online",
		Ready: true, Calibrated: true, FrictionReady: true,
		CalibrationSamples: 1, ExpectedReturn: 0.05, ReferencePrice: 100,
		BuyCapacity: 100, SellCapacity: 100, ExpectedSpread: 0.001,
		ExpectedImpact: 0.001, ExpectedAdverseSelection: 0.001,
		Confidence: 1,
	}
	fees := map[string]float64{"BTC/USD": 0.001}

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, forecast)
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Source: "dmt", Symbol: "BTC/USD", At: forecast.At,
			Ready: true, Winner: "buy",
		})
		planner.Decide(thesis, fees, 100, 1)
	}
}

func BenchmarkPlanner_UpdateMeasuresSignals(b *testing.B) {
	values := make([]float64, 65536)

	for index := range values {
		values[index] = float64(index + 1)
	}

	signals := make([]types.Signal, 11)

	for index := range signals {
		signals[index] = &plannerWorkSignal{values: values}
	}

	planner := NewPlanner(context.Background(), nil, signals, nil)
	b.Cleanup(planner.Close)

	b.Run("sequential", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			thesis := types.NewThesis(nil)

			for _, signal := range signals {
				signal.Measure(thesis)
			}
		}
	})

	b.Run("channel", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			planner.Update("", "")
		}
	})
}
