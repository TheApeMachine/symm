package types

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestThesisPublish(t *testing.T) {
	Convey("Given a thesis without publishable tick evidence", t, func() {
		uiHub := make(chan []byte, 1)
		thesis := NewThesis(uiHub)

		thesis.Publish()

		Convey("Then it does not consume UI capacity with an empty frame", func() {
			So(len(uiHub), ShouldEqual, 0)
		})
	})

	Convey("Given a thesis carrying signal measurements and logic results", t, func() {
		uiHub := make(chan []byte, 1)
		thesis := NewThesis(uiHub)

		thesis.Measurements = append(thesis.Measurements, &Measurement{
			Source: SourcePumpDump, Metric: MetricStrength, Symbol: "BTC/USD", At: time.Unix(1, 0), Raw: 0.7,
		})
		thesis.Graphs["BTC/USD"] = NewGraph("BTC/USD")
		thesis.Forecasts = append(thesis.Forecasts, Forecasts{
			Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		})
		thesis.Hypotheses = append(thesis.Hypotheses, Hypothesis{
			Source: SourceCausal, Symbol: "BTC/USD", At: time.Unix(1, 0),
		})
		So(thesis.Transition(
			"BTC/USD", LifecycleShaped, time.Unix(1, 0),
		), ShouldBeNil)

		Convey("When published", func() {
			thesis.Publish()

			Convey("Then the thesis evidence reaches the wire", func() {
				payload := <-uiHub

				var frame struct {
					Measurements []Measurement      `json:"measurements"`
					TradeJournal []TradeObservation `json:"tradeJournal"`
					Lifecycle    map[string]string  `json:"lifecycle"`
					Graphs       []GraphFrame       `json:"graphs"`
					Forecasts    []Forecasts        `json:"forecasts"`
					Hypotheses   []Hypothesis       `json:"hypotheses"`
				}

				So(json.Unmarshal(payload, &frame), ShouldBeNil)
				So(len(frame.Measurements), ShouldEqual, 1)
				So(frame.Measurements[0].Symbol, ShouldEqual, "BTC/USD")
				So(frame.Measurements[0].Source, ShouldEqual, SourcePumpDump)
				So(frame.TradeJournal, ShouldHaveLength, 1)
				So(frame.Lifecycle["BTC/USD"], ShouldEqual, LifecycleShaped)
				So(frame.Graphs, ShouldHaveLength, 1)
				So(frame.Graphs[0].Symbol, ShouldEqual, "BTC/USD")
				So(frame.Forecasts, ShouldHaveLength, 1)
				So(frame.Hypotheses, ShouldHaveLength, 1)
			})
		})
	})

	Convey("Given a tick without decision or lifecycle events", t, func() {
		uiHub := make(chan []byte, 1)
		thesis := NewThesis(uiHub)
		thesis.Tick = 1

		thesis.Publish()

		Convey("Then absent event collections are not published as clearing frames", func() {
			frame := map[string]any{}
			So(json.Unmarshal(<-uiHub, &frame), ShouldBeNil)
			So(frame, ShouldNotContainKey, "decisions")
			So(frame, ShouldNotContainKey, "tradeJournal")
			So(frame, ShouldNotContainKey, "lifecycle")
			So(frame, ShouldNotContainKey, "manifold")
		})
	})

	Convey("Given a tick thesis absorbing evaluated PostMortem findings", t, func() {
		uiHub := make(chan []byte, 1)
		tickThesis := NewThesis(uiHub)
		evaluated := NewThesis(nil)
		evaluated.Findings = append(evaluated.Findings, Finding{
			Symbol: "BTC/USD", Component: "execution",
			Condition: "entry and exit fills reconciled with reported fees",
			Evidence:  []string{"buy-fill", "sell-fill"}, EstimatedEffect: 7.92,
			RequiredValidation: "aggregate comparable completed Theses and validate by chronological replay",
		})

		tickThesis.AbsorbFindings(evaluated)
		tickThesis.Publish()

		Convey("Then the wire frame carries the structured symbol", func() {
			var frame struct {
				Findings []Finding `json:"findings"`
			}

			So(json.Unmarshal(<-uiHub, &frame), ShouldBeNil)
			So(frame.Findings, ShouldHaveLength, 1)
			So(frame.Findings[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func BenchmarkThesisPublish(b *testing.B) {
	uiHub := make(chan []byte, 1)
	thesis := NewThesis(uiHub)
	thesis.Measurements = append(thesis.Measurements, &Measurement{
		Source: SourcePumpDump, Metric: MetricStrength,
		Symbol: "BTC/USD", At: time.Unix(1, 0), Raw: 0.7,
	})

	b.ReportAllocs()

	for b.Loop() {
		thesis.Publish()
		<-uiHub
	}
}

func BenchmarkThesisAbsorb(b *testing.B) {
	current := NewThesis(nil)

	for index := 0; index < 32; index++ {
		current.Measurements = append(current.Measurements, &Measurement{
			Symbol: "BTC/USD", Raw: float64(index),
		})
	}

	current.Forecasts = append(current.Forecasts, Forecasts{Symbol: "BTC/USD"})
	current.Hypotheses = append(current.Hypotheses, Hypothesis{Symbol: "BTC/USD"})
	current.Categories = append(current.Categories, Category{Symbol: "BTC/USD"})
	current.Graphs["BTC/USD"] = NewGraph("BTC/USD")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		lifecycle := NewThesis(nil)
		lifecycle.Absorb(current, "BTC/USD")

		if len(lifecycle.Measurements) != 32 {
			b.Fatal("symbol evidence was not absorbed")
		}
	}
}

func TestThesisAbsorb(t *testing.T) {
	Convey("Given an open-position Thesis and a later market tick", t, func() {
		lifecycle := NewThesis(nil)
		current := NewThesis(nil)
		current.Measurements = []*Measurement{
			{Symbol: "BTC/USD", Raw: 1},
			{Symbol: "ETH/USD", Raw: 2},
		}
		current.Forecasts = []Forecasts{
			{Symbol: "BTC/USD", ExpectedReturn: 0.01},
			{Symbol: "ETH/USD", ExpectedReturn: 0.02},
		}
		current.Hypotheses = []Hypothesis{
			{Symbol: "BTC/USD", Claim: "continuation"},
			{Symbol: "ETH/USD", Claim: "unrelated"},
		}
		current.Categories = []Category{
			{Symbol: "BTC/USD", Type: OrganicTrend},
			{Symbol: "ETH/USD", Type: Equilibrium},
		}
		current.Graphs["BTC/USD"] = NewGraph("BTC/USD")

		lifecycle.Absorb(current, "BTC/USD")

		Convey("Then only evidence used to manage that position is retained", func() {
			So(lifecycle.Measurements, ShouldHaveLength, 1)
			So(lifecycle.Forecasts, ShouldHaveLength, 1)
			So(lifecycle.Hypotheses, ShouldHaveLength, 1)
			So(lifecycle.Categories, ShouldHaveLength, 1)
			So(lifecycle.Graphs, ShouldContainKey, "BTC/USD")
			So(lifecycle.Measurements[0].Symbol, ShouldEqual, "BTC/USD")
		})

		Convey("Then repeated absorption of the same tick stays idempotent", func() {
			lifecycle.Absorb(current, "BTC/USD")

			So(lifecycle.Measurements, ShouldHaveLength, 1)
			So(lifecycle.Forecasts, ShouldHaveLength, 1)
			So(lifecycle.Hypotheses, ShouldHaveLength, 1)
			So(lifecycle.Categories, ShouldHaveLength, 1)
		})
	})
}

func TestThesisTransition(t *testing.T) {
	Convey("Given a newly observed symbol", t, func() {
		thesis := NewThesis(nil)

		Convey("Then valid trade lifecycle edges advance in order", func() {
			So(thesis.Transition("BTC/USD", LifecycleShaped, time.Unix(1, 0)), ShouldBeNil)
			So(thesis.Transition("BTC/USD", LifecycleEntrySelected, time.Unix(2, 0)), ShouldBeNil)
			So(thesis.Transition("BTC/USD", LifecycleEntrySubmitted, time.Unix(3, 0)), ShouldBeNil)
			So(thesis.Transition("BTC/USD", LifecycleEntered, time.Unix(4, 0)), ShouldBeNil)
			So(thesis.Transition("BTC/USD", LifecycleManaging, time.Unix(5, 0)), ShouldBeNil)
			So(thesis.Transition("BTC/USD", LifecycleExitSelected, time.Unix(6, 0)), ShouldBeNil)
			So(thesis.Transition("BTC/USD", LifecycleExitSubmitted, time.Unix(7, 0)), ShouldBeNil)
			So(thesis.Transition("BTC/USD", LifecycleClosed, time.Unix(8, 0)), ShouldBeNil)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, LifecycleClosed)
			So(thesis.TradeJournal, ShouldHaveLength, 8)
			So(thesis.TradeJournal[7].At, ShouldResemble, time.Unix(8, 0))
		})

		Convey("Then an impossible edge is rejected without changing state", func() {
			err := thesis.Transition("BTC/USD", LifecycleClosed, time.Unix(1, 0))

			So(err, ShouldNotBeNil)
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, LifecycleObserving)
		})
	})
}

func TestThesisObservePostExit(t *testing.T) {
	Convey("Given a closed lifecycle with a two-epoch forecast horizon", t, func() {
		thesis := NewThesis(nil)
		thesis.Forecasts = append(thesis.Forecasts, Forecasts{
			Symbol: "BTC/USD", At: time.Unix(2, 0), SourceEpoch: 10, HorizonEvents: 2,
		})
		So(thesis.Transition("BTC/USD", LifecycleManaging, time.Unix(1, 0)), ShouldBeNil)
		So(thesis.Transition("BTC/USD", LifecycleExitSelected, time.Unix(2, 0)), ShouldBeNil)
		So(thesis.Transition("BTC/USD", LifecycleExitSubmitted, time.Unix(2, 0)), ShouldBeNil)
		So(thesis.Transition("BTC/USD", LifecycleClosed, time.Unix(3, 0)), ShouldBeNil)

		first := NewThesis(nil)
		first.Forecasts = append(first.Forecasts, Forecasts{
			Symbol: "BTC/USD", At: time.Unix(4, 0), SourceEpoch: 11,
		})
		second := NewThesis(nil)
		second.Forecasts = append(second.Forecasts, Forecasts{
			Symbol: "BTC/USD", At: time.Unix(5, 0), SourceEpoch: 12,
		})

		So(thesis.ObservePostExit(first, "BTC/USD"), ShouldBeNil)

		Convey("Then one epoch starts but does not complete the required tail", func() {
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, LifecyclePostExitObservation)
		})

		So(thesis.ObservePostExit(first, "BTC/USD"), ShouldBeNil)

		Convey("Then repeating the same post-exit tick does not duplicate evidence", func() {
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, LifecyclePostExitObservation)
			So(thesis.Forecasts, ShouldHaveLength, 2)
		})

		So(thesis.ObservePostExit(second, "BTC/USD"), ShouldBeNil)

		Convey("Then the second distinct epoch makes the Thesis PostMortem-ready", func() {
			So(thesis.LifecycleState("BTC/USD"), ShouldEqual, LifecyclePostMortemReady)
			So(thesis.Forecasts, ShouldHaveLength, 3)
		})
	})
}
