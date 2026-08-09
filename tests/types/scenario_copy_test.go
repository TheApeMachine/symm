package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScenarioConfigClone(t *testing.T) {
	Convey("Given a scenario with every mutable collection populated", t, func() {
		config := NewScenarioConfig([]*Symbol{NewSymbol("COPY/USD", 100, 51)})
		config.Schedule = []RegimeTransition{{
			Tick: 1, Symbol: "COPY/USD", State: FastPump,
		}}
		config.Execution.Outcomes = []OrderOutcome{OrderFill}
		config.Faults.ChannelLatency = map[string]LatencyConfig{
			"ticker": {Base: time.Millisecond},
		}
		config.Faults.Rules = []FaultRule{{
			Channel: "ticker", Occurrence: 1, Action: FaultMalformed,
			Payload: []byte("original"),
		}}
		config.InitialBalances = map[string]float64{"USD": 1_000}
		clone := config.Clone()
		config.Symbols[0].StartPrice = 200
		profile := config.Profiles[FastPump]
		profile.Precursor.Metrics[0].MinimumNormalized = 99
		config.Profiles[FastPump] = profile
		config.Momentum[FastPump] = 99
		config.Schedule[0].Tick = 2
		config.Execution.Outcomes[0] = OrderReject
		config.Faults.ChannelLatency["ticker"] = LatencyConfig{Base: time.Second}
		config.Faults.Rules[0].Payload[0] = 'X'
		config.InitialBalances["USD"] = 0

		Convey("Caller mutation should not change the replay copy", func() {
			So(clone.Symbols[0].StartPrice, ShouldEqual, 100.0)
			So(clone.Profiles[FastPump].Precursor.Metrics[0].MinimumNormalized,
				ShouldNotEqual, 99.0)
			So(clone.Momentum[FastPump], ShouldNotEqual, 99.0)
			So(clone.Schedule[0].Tick, ShouldEqual, uint64(1))
			So(clone.Execution.Outcomes[0], ShouldEqual, OrderFill)
			So(clone.Faults.ChannelLatency["ticker"].Base,
				ShouldEqual, time.Millisecond)
			So(string(clone.Faults.Rules[0].Payload), ShouldEqual, "original")
			So(clone.InitialBalances["USD"], ShouldEqual, 1_000.0)
		})
	})
}

func BenchmarkScenarioConfigClone(b *testing.B) {
	config := NewScenarioConfig([]*Symbol{
		NewSymbol("COPY1/USD", 100, 51),
		NewSymbol("COPY2/USD", 200, 52),
	})
	config.Faults.Rules = []FaultRule{{
		Channel: "ticker", Occurrence: 1, Action: FaultMalformed,
		Payload: []byte("{"),
	}}

	for b.Loop() {
		_ = config.Clone()
	}
}
