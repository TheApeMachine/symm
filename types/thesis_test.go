package types

import (
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestResetTick proves that a new market cut keeps only selected entry intent and
durable lifecycle state while replacing every tick-local evidence surface.
*/
func TestResetTick(t *testing.T) {
	at := time.Unix(20, 0).UTC()
	decisions := []struct {
		name     string
		decision Decision
		phase    string
		retained bool
	}{
		{
			name: "selected enter remains pending",
			decision: Decision{
				Action:            ActionEnter,
				Symbol:            "BTC/USD",
				Utility:           0.81,
				ValidThroughEpoch: 19,
			},
			phase:    LifecycleEntrySelected,
			retained: true,
		},
		{
			name: "submitted enter is no longer planner pending",
			decision: Decision{
				Action: ActionEnter,
				Symbol: "ETH/USD",
			},
			phase: LifecycleEntrySubmitted,
		},
		{
			name: "selected hold is not entry intent",
			decision: Decision{
				Action: ActionHold,
				Symbol: "SOL/USD",
			},
			phase: LifecycleEntrySelected,
		},
		{
			name: "enter without selected lifecycle is tick local",
			decision: Decision{
				Action: ActionEnter,
				Symbol: "XRP/USD",
			},
		},
	}

	for _, scenario := range decisions {
		scenario := scenario

		Convey("Given a "+scenario.name, t, func() {
			thesis := NewThesis(nil)
			thesis.Decisions = []Decision{scenario.decision}

			if scenario.phase != "" {
				thesis.Lifecycle.Store(scenario.decision.Symbol, scenario.phase)
			}

			thesis.ResetTick(at, 20)

			if scenario.retained {
				So(thesis.Decisions, ShouldResemble, []Decision{scenario.decision})
				return
			}

			So(thesis.Decisions, ShouldResemble, []Decision{})
		})
	}

	Convey("Given durable state and a fully populated market cut", t, func() {
		thesis := NewThesis(nil)
		holding := &Holding{Symbol: "BTC/USD", Status: OPEN}
		finding := Finding{Symbol: "BTC/USD", Component: "strategy"}
		previousCrossSection := thesis.CrossSection
		thesis.Tick = 19
		thesis.Measurements = []*Measurement{{Symbol: "BTC/USD"}}
		thesis.Forecasts = []Forecasts{{Symbol: "BTC/USD"}}
		thesis.Hypotheses = []Hypothesis{{Symbol: "BTC/USD"}}
		thesis.Categories = []Category{{Symbol: "BTC/USD"}}
		thesis.Resonance = []any{"resonance"}
		thesis.Causal = []any{"causal"}
		thesis.Findings = []Finding{finding}
		thesis.CrossSection.Metrics.Store("BTC/USD", SymbolMetric{Symbol: "BTC/USD"})
		thesis.Graphs.Store("BTC/USD", NewGraph("BTC/USD"))
		thesis.Manifold.Store("BTC/USD", "manifold")
		thesis.Cognition.Store("BTC/USD", Cognition{Symbol: "BTC/USD"})
		thesis.Positions.Store("BTC/USD", true)
		thesis.Holdings.Store("BTC/USD", holding)
		thesis.Lifecycle.Store("BTC/USD", LifecycleManaging)
		thesis.NoteIncomplete()

		thesis.ResetTick(at, 20)

		So(thesis.Tick, ShouldEqual, int64(20))
		So(thesis.At, ShouldResemble, at)
		So(thesis.CrossSection, ShouldNotEqual, previousCrossSection)
		_, crossSectionFound := thesis.CrossSection.Metrics.Load("BTC/USD")
		So(crossSectionFound, ShouldBeFalse)
		So(thesis.Measurements, ShouldBeNil)
		So(thesis.Forecasts, ShouldResemble, []Forecasts{})
		So(thesis.Hypotheses, ShouldResemble, []Hypothesis{})
		So(thesis.Categories, ShouldResemble, []Category{})
		So(thesis.Resonance, ShouldResemble, []any{})
		So(thesis.Causal, ShouldResemble, []any{})
		So(thesis.Incomplete(), ShouldBeFalse)

		for _, values := range []*sync.Map{
			thesis.Graphs,
			thesis.Manifold,
			thesis.Cognition,
			thesis.Positions,
		} {
			count := 0
			values.Range(func(_, _ any) bool {
				count++
				return true
			})
			So(count, ShouldEqual, 0)
		}

		storedHolding, holdingFound := thesis.Holdings.Load("BTC/USD")
		So(holdingFound, ShouldBeTrue)
		So(storedHolding, ShouldEqual, holding)
		phase, phaseFound := thesis.Lifecycle.Load("BTC/USD")
		So(phaseFound, ShouldBeTrue)
		So(phase, ShouldEqual, LifecycleManaging)
		So(thesis.Findings, ShouldResemble, []Finding{finding})
	})
}

/*
BenchmarkResetTick measures replacement of one populated 128-symbol exchange
cut with four selected entries while excluding fixture reconstruction from the
timed reset operation.
*/
func BenchmarkResetTick(b *testing.B) {
	const symbolCount = 128
	const selectedEntries = 4

	thesis := NewThesis(nil)
	at := time.Unix(20, 0).UTC()
	symbols := make([]string, symbolCount)
	decisions := make([]Decision, symbolCount)
	seedDecisions := make([]Decision, symbolCount)
	measurements := make([]*Measurement, symbolCount)
	forecasts := make([]Forecasts, symbolCount)

	for index := range symbolCount {
		symbol := "ASSET-" + strconv.Itoa(index) + "/USD"
		symbols[index] = symbol
		seedDecisions[index] = Decision{Action: ActionHold, Symbol: symbol}
		measurements[index] = &Measurement{Symbol: symbol}
		forecasts[index] = Forecasts{Symbol: symbol}

		if index < selectedEntries {
			seedDecisions[index].Action = ActionEnter
			thesis.Lifecycle.Store(symbol, LifecycleEntrySelected)
		}
	}

	b.ReportAllocs()
	var tick int64

	for b.Loop() {
		b.StopTimer()
		copy(decisions, seedDecisions)
		thesis.Decisions = decisions
		thesis.Measurements = measurements
		thesis.Forecasts = forecasts

		for _, symbol := range symbols {
			thesis.Graphs.Store(symbol, NewGraph(symbol))
			thesis.Manifold.Store(symbol, symbol)
			thesis.Cognition.Store(symbol, Cognition{Symbol: symbol})
			thesis.Positions.Store(symbol, true)
		}

		tick++
		b.StartTimer()
		thesis.ResetTick(at, tick)
	}
}

func TestNoteLifecycle(t *testing.T) {
	Convey("Given a thesis phase transition", t, func() {
		thesis := NewThesis(nil)
		at := time.Unix(1, 0).UTC()
		thesis.NoteLifecycle("BTC/USD", LifecycleEntered, at)

		Convey("It should store the phase without a parallel journal", func() {
			phase, ok := thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, LifecycleEntered)
		})
	})
}

func BenchmarkNoteLifecycle(b *testing.B) {
	thesis := NewThesis(nil)
	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, at)
	}
}
