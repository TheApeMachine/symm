package strategy

import (
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"golang.org/x/sync/errgroup"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func TestLearnerMarshalFlatbuffer(t *testing.T) {
	Convey("The wire carries original grid identities and signed agent readings", t, func() {
		learner, _ := learningFixture(t)

		for index, symbol := range []string{"BTC/USD", "ETH/USD"} {
			measurement := data.NewMeasurement[float64](
				"wire", symbol, "signal", time.Now(), time.Now(),
			)

			measurement.PutMetric(data.Metric[float64]{
				Label: symbol,
				Raw:   float64(index),
			})

			learner.Step(learner.Grid.Step(&types.Envelope{CVD: measurement}))
		}

		learner.Agent.Reading.Defined = true
		learner.Agent.Reading.Mean = -.125

		state := wire.GetRootAsLearningState(
			learner.MarshalFlatbuffer("BTC/USD"), 0,
		).UnPack()

		So(state.Steps, ShouldEqual, 0) // A grid can be displayed before agents are ready.
		So(state.Markets, ShouldHaveLength, 2)
		So(state.Markets[0].Quantities, ShouldHaveLength, 2)
		So(state.Markets[1].Quantities, ShouldBeEmpty)
		So(state.Agents[0].Reading.Mean, ShouldEqual, -.125)
		So(state.Agents[0].Cash, ShouldEqual, learner.Traders[0].Balance.Cash().String())
	})
}

func BenchmarkLearnerMarshalFlatbuffer(b *testing.B) {
	learner, _ := learningFixture(b)
	measurement := data.NewMeasurement[float64]("wire", "BTC/USD", "signal", time.Now(), time.Now())

	for index := range 800 {
		measurement.PutMetric(data.Metric[float64]{
			Label: string(rune(index + 256)),
			Raw:   float64(index),
		})
	}

	learner.Step(learner.Grid.Step(&types.Envelope{CVD: measurement}))
	b.ReportAllocs()

	for b.Loop() {
		learner.MarshalFlatbuffer("BTC/USD")
	}
}

func TestRehearsalWire(t *testing.T) {
	Convey("Historical counters travel separately from live decisions and outcomes", t, func() {
		learner, _ := learningFixture(t)
		learner.Rehearsal.progress.Decisions = 20
		learner.Rehearsal.progress.Trained = 19
		learner.Rehearsal.progress.Ungraded = 3
		learner.Rehearsal.progress.LastReturn = -0.02
		state := wire.GetRootAsLearningState(learner.MarshalFlatbuffer("BTC/USD"), 0).UnPack()
		So(state.Rehearsal.Decisions, ShouldEqual, 20)
		So(state.Rehearsal.Trained, ShouldEqual, 19)
		So(state.Rehearsal.Ungraded, ShouldEqual, 3)
		So(state.Rehearsal.LastReturn, ShouldEqual, -0.02)
		So(state.Decisions, ShouldEqual, 0)
		So(state.Resolved, ShouldEqual, 0)
	})
}

func TestLearnerMarshalFlatbufferConcurrent(t *testing.T) {
	Convey("Live updates, replay identity admission and telemetry share grid ownership", t, func() {
		learner, _ := learningFixture(t)
		var workers errgroup.Group
		workers.Go(func() error {
			for index := range 32 {
				measurement := data.NewMeasurement[float64]("wire", "BTC/USD", "signal", time.Now(), time.Now())
				measurement.PutMetric(data.Metric[float64]{Label: "change", Raw: float64(index % 3)})
				learner.Step(learner.Grid.Step(&types.Envelope{CVD: measurement}))
			}
			return learner.Error()
		})
		workers.Go(func() error {
			for index := range 32 {
				columns := [][2]string{{"replay", strconv.Itoa(index)}}

				if err := learner.Learn("BTC/USD", columns,
					[]uint64{FlatPositionContext, grid.ConditionToken(1, 1, -1)}, Action{Kind: "enter"}, -0.01, 0.5); err != nil {
					return err
				}
			}
			return nil
		})
		workers.Go(func() error {
			for range 32 {
				wire.GetRootAsLearningState(learner.MarshalFlatbuffer("BTC/USD"), 0).UnPack()
			}
			return nil
		})
		So(workers.Wait(), ShouldBeNil)
	})
}
