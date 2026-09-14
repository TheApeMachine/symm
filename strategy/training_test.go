package strategy

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/types"
)

func ringChildLen[T any](ring core.Primitive) int {
	result, err := ringCommand(ring, &store.RingCommand[T]{ChildLen: true})

	if err != nil {
		return 0
	}

	return result.ChildLen
}

func TestTrainingStep(t *testing.T) {
	Convey("An empty pipeline returns the measurement without panicking", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)
		measurement := data.NewMeasurement("training", map[string]data.Metric[float64]{})
		So(training.Step(measurement), ShouldEqual, measurement)
		So(training.Error(), ShouldBeNil)
	})

	Convey("A tape frame flows through the nomagique composition", t, func() {
		tape := NewTape()
		measurement := data.NewMeasurement[float64]("cvd", nil)
		measurement.Label, measurement.At, measurement.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurement.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.5}
		measurement2 := data.NewMeasurement[float64]("cvd", nil)
		measurement2.Label, measurement2.At, measurement2.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurement2.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.8}
		tape.Publish(types.ReplayFragment{
			Frames:      [][]*data.Measurement[float64]{{measurement}, {measurement2}},
			Symbol:      "BTC/USD",
			AnchorIndex: 1,
		})
		tape.Close()
		training := NewTraining(context.Background(), tape)
		input := data.NewMeasurement("training", map[string]data.Metric[float64]{})
		So(training.Step(input), ShouldEqual, input)
		So(spaceState(training.agents[1].Space()).Updated, ShouldEqual, "BTC/USD")
		So(spaceState(training.space).Updated, ShouldEqual, "")
		So(training.Error(), ShouldBeNil)
	})

	Convey("Multi-agent population instantiates 8 traders by default and serializes all learners", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)
		So(len(training.agents), ShouldEqual, 8)
		So(training.agents[0].IsLive(), ShouldBeTrue)
		So(training.agents[1].IsLive(), ShouldBeFalse)

		state := training.State().state
		So(state, ShouldNotBeNil)
		So(len(state.Agents), ShouldEqual, 8)
		So(state.Agents[0].Initial, ShouldStartWith, "200")
		So(state.Agents[0].Cash, ShouldStartWith, "200")
		So(state.Agents[0].Status, ShouldEqual, "paper")
		So(state.Agents[1].Status, ShouldEqual, "learning")
		So(state.Rehearsal, ShouldNotBeNil)
		So(state.Rehearsal.Workers, ShouldEqual, 8)
		So(len(state.Rehearsal.Tracks), ShouldEqual, 8)
		So(len(state.Recognition.Learners), ShouldEqual, 8)
	})

	Convey("Live step with empty symbol falls back to measurement label and updates state", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)
		cvd := data.NewMeasurement("cvd", map[string]data.Metric[float64]{})
		cvd.Label, cvd.At, cvd.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
		measurement := data.NewMeasurement("training", map[string]data.Metric[float64]{})
		measurement.Peers = []*data.Measurement[float64]{cvd}
		So(training.Step(measurement), ShouldEqual, measurement)
		So(spaceState(training.space).Updated, ShouldEqual, "BTC/USD")
		So(training.Error(), ShouldBeNil)
	})

	Convey("Live step ignores peer measurements that carry errors without poisoning perception grid", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)
		erroredPeer := data.NewMeasurement("hawkes", map[string]data.Metric[float64]{})
		erroredPeer.Label, erroredPeer.At, erroredPeer.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
		erroredPeer.Err = core.ErrDomain
		validPeer := data.NewMeasurement("cvd", map[string]data.Metric[float64]{})
		validPeer.Label, validPeer.At, validPeer.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
		validPeer.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.0}

		measurement := data.NewMeasurement("training", map[string]data.Metric[float64]{})
		measurement.Peers = []*data.Measurement[float64]{erroredPeer, validPeer}
		So(training.Step(measurement), ShouldEqual, measurement)
		So(training.Error(), ShouldBeNil)
		So(spaceState(training.space).Updated, ShouldEqual, "BTC/USD")
	})

	Convey("Ring-of-rings rehearsal ingests fragments with random slots and plays with random offsets", t, func() {
		agent := NewAgent(1, false, nil, 64)
		measurementA := data.NewMeasurement[float64]("cvd", nil)
		measurementA.Label, measurementA.At, measurementA.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurementA.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.0}

		measurementB := data.NewMeasurement[float64]("cvd", nil)
		measurementB.Label, measurementB.At, measurementB.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measurementB.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 2.0}

		fragment := types.ReplayFragment{
			Frames: [][]*data.Measurement[float64]{
				{measurementA},
				{measurementB},
			},
			Symbol:      "BTC/USD",
			AnchorIndex: 1,
		}

		agent.IngestReplay(fragment, 0)
		So(ringLen[[]*data.Measurement[float64]](agent.ring), ShouldEqual, 1)
		So(ringChildLen[[]*data.Measurement[float64]](agent.ring), ShouldEqual, 2)

		stepped, err := agent.RehearseChild()
		So(err, ShouldBeNil)
		So(stepped, ShouldBeGreaterThanOrEqualTo, 1)
	})

	Convey("Focus restricts quantities and regions payload to the focused symbol", t, func() {
		tape := NewTape()
		tape.Close()
		training := NewTraining(context.Background(), tape)

		cvdBTC := data.NewMeasurement("cvd", map[string]data.Metric[float64]{
			"signed": data.NewMetric[float64]("signed", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(1.0),
		})
		cvdBTC.Label, cvdBTC.At, cvdBTC.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
		mBTC := data.NewMeasurement("training", map[string]data.Metric[float64]{})
		mBTC.Peers = []*data.Measurement[float64]{cvdBTC}
		training.Step(mBTC)

		cvdETH := data.NewMeasurement("cvd", map[string]data.Metric[float64]{
			"signed": data.NewMetric[float64]("signed", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(2.0),
		})
		cvdETH.Label, cvdETH.At, cvdETH.From = "ETH/USD", time.Now().UTC(), time.Now().UTC()
		mETH := data.NewMeasurement("training", map[string]data.Metric[float64]{})
		mETH.Peers = []*data.Measurement[float64]{cvdETH}
		training.Step(mETH)

		recBTC := training.State("BTC/USD")
		So(recBTC, ShouldNotBeNil)
		So(len(recBTC.state.Markets), ShouldEqual, 2)
		for _, market := range recBTC.state.Markets {
			if market.Symbol == "BTC/USD" {
				So(len(market.Quantities), ShouldBeGreaterThan, 0)
			}
			if market.Symbol == "ETH/USD" {
				So(len(market.Quantities), ShouldEqual, 0)
			}
		}

		recETH := training.State("ETH/USD")
		So(recETH, ShouldNotBeNil)
		So(len(recETH.state.Markets), ShouldEqual, 2)
		for _, market := range recETH.state.Markets {
			if market.Symbol == "ETH/USD" {
				So(len(market.Quantities), ShouldBeGreaterThan, 0)
			}
			if market.Symbol == "BTC/USD" {
				So(len(market.Quantities), ShouldEqual, 0)
			}
		}

		payload := training.MarshalFlatbuffer("BTC/USD")
		So(len(payload), ShouldBeGreaterThan, 0)
	})

	Convey("Negative and non-event fragments can be published and mounted (AT-13, AT-14)", t, func() {
		tape := NewTape()
		measA := data.NewMeasurement[float64]("cvd", nil)
		measA.Label, measA.At, measA.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measA.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 0.1}
		measB := data.NewMeasurement[float64]("cvd", nil)
		measB.Label, measB.At, measB.From = "BTC/USD", time.Now().UTC(), time.Time{}
		measB.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 0.05}

		tape.Publish(types.ReplayFragment{
			Frames:        [][]*data.Measurement[float64]{{measA}, {measB}},
			Symbol:        "BTC/USD",
			AnchorIndex:   -1,
			ExtremumIndex: -1,
		})
		tape.Close()

		training := NewTraining(context.Background(), tape)
		input := data.NewMeasurement("training", map[string]data.Metric[float64]{})
		So(training.Step(input), ShouldEqual, input)
		So(training.Error(), ShouldBeNil)
		So(len(training.legs), ShouldEqual, 1)
	})
}

