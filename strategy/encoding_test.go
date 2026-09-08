package strategy

import (
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

			learner.Step(&types.Envelope{CVD: measurement})
		}

		learner.Population.Agents[0].Reading.Defined = true
		learner.Population.Agents[0].Reading.Mean = -.125

		state := wire.GetRootAsLearningState(
			learner.MarshalFlatbuffer("BTC/USD"), 0,
		).UnPack()

		So(state.Steps, ShouldEqual, 2)
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

	learner.Step(&types.Envelope{CVD: measurement})
	b.ReportAllocs()

	for b.Loop() {
		learner.MarshalFlatbuffer("BTC/USD")
	}
}
