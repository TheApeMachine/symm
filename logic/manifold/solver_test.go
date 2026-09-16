package manifold

import (
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestSolverPublishReading(t *testing.T) {
	Convey("Given a dataset and a spot book with orders", t, func() {
		ds := NewDataset()
		bk := spotbook.New()

		// Insert a bid and ask
		bk.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Bid,
			ID:        "bid-1",
			Price:     decimal.NewFromFloat64(50000.0),
			Quantity:  decimal.NewFromFloat64(1.5),
			Timestamp: time.Now(),
		})
		bk.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Bid,
			ID:        "bid-2",
			Price:     decimal.NewFromFloat64(49990.0),
			Quantity:  decimal.NewFromFloat64(2.0),
			Timestamp: time.Now(),
		})
		bk.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Ask,
			ID:        "ask-1",
			Price:     decimal.NewFromFloat64(50010.0),
			Quantity:  decimal.NewFromFloat64(1.0),
			Timestamp: time.Now(),
		})

		var states []*sensorium.State
		for state := range ds.Step("BTC/USD", bk.Bids, bk.Asks, forcingState{}) {
			states = append(states, state)
		}

		So(ds.Error(), ShouldBeNil)
		So(len(states), ShouldEqual, 3)

		Convey("And a solver stepping this state", func() {
			physics := sensorium.NewManifold(8, 8, 8)
			solver := &Solver{
				System:  runtime.NewSystem(t.Context(), "manifold"),
				physics: physics,
				dataset: ds,
				loaded:  make(map[int64]struct{}),
				dirty:   make(map[string]struct{}),
				wake:    make(chan struct{}, 1),
			}
			solver.Transition(runtime.READY)
			defer func() { So(solver.Close(), ShouldBeNil); So(physics.Close(), ShouldBeNil) }()

			batch := collectStates(states)
			So(batch, ShouldNotBeNil)
			So(batch.N, ShouldEqual, 3)

			solver.advanceMu.Lock()
			stepped, err := solver.physics.Step(batch)
			So(err, ShouldBeNil)
			So(stepped, ShouldNotBeNil)
			reading := solver.publishReading(stepped)
			solver.advanceMu.Unlock()

			So(reading, ShouldNotBeNil)
			So(reading.Reading.CoherenceMag2, ShouldBeGreaterThanOrEqualTo, 0)

			measurement := data.NewMeasurement("manifold", map[string]data.Metric[float64]{
				"coherence_mag2": data.NewMetric[float64](
					"coherence_mag2", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
				),
				"particle_count": data.NewMetric[float64](
					"particle_count", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
				),
			})
			measurement.Label = "BTC/USD"

			steppedMeasurement := solver.Step(measurement)
			So(steppedMeasurement, ShouldNotBeNil)
			So(steppedMeasurement.Metrics["coherence_mag2"].Raw, ShouldEqual, reading.Reading.CoherenceMag2)
			So(steppedMeasurement.Metrics["particle_count"].Raw, ShouldEqual, float64(reading.State.N))

			So(reading.GridX, ShouldEqual, 8)
			So(len(reading.MomRho), ShouldEqual, 8*8*8*4)
			originalPosition := reading.State.Pos[0]
			stepped.Pos[0]++
			So(reading.State.Pos[0], ShouldEqual, originalPosition)
			next := solver.publishReading(stepped)
			So(next.Version, ShouldEqual, reading.Version+1)
			So(next.State.Pos[0], ShouldEqual, stepped.Pos[0])
			next.MomRho[0]++
			So(next.MomRho[0], ShouldNotEqual, reading.MomRho[0])

		})

	})
}

func TestSolverStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Solver{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(node.Step(measurement), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func TestSolverStepArtifact(t *testing.T) {
	Convey("Step publishes a complete physics frame through its measurement", t, func() {
		physics := sensorium.NewManifold(8, 8, 8)
		defer func() { So(physics.Close(), ShouldBeNil) }()
		solver := &Solver{
			System:  runtime.NewSystem(t.Context(), "manifold-artifact"),
			physics: physics,
		}
		solver.Transition(runtime.READY)
		reading := solver.publishReading(physics.State())
		measurement := solver.Register()

		result := solver.Step(measurement)
		So(result, ShouldEqual, measurement)
		So(reading.GridX, ShouldEqual, 8)
		So(reading.GridY, ShouldEqual, 8)
		So(reading.GridZ, ShouldEqual, 8)
		So(reading.GridSpacing, ShouldBeGreaterThan, 0)
		So(len(reading.MomRho), ShouldEqual, 8*8*8*4)
		So(len(reading.FieldEnergy), ShouldEqual, 8*8*8)
		So(len(reading.WaveReal), ShouldEqual, 8*8*8)
		So(len(reading.WaveImag), ShouldEqual, 8*8*8)
	})
}

// BenchmarkSolverPublishReading measures full production-sized grid publication.
func BenchmarkSolverPublishReading(b *testing.B) {
	physics := sensorium.NewManifold(64, 64, 64)
	defer func() {
		if err := physics.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	solver := &Solver{physics: physics}
	state := physics.State()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		solver.publishReading(state)
	}
}
