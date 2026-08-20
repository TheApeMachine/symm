package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"
)

func TestThesisMarshalState(t *testing.T) {
	Convey("Given a thesis carrying symbol and equity state", t, func() {
		thesis := NewThesis(t.Context(), nil)
		thesis.Tick = 7
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		symbol := thesis.Symbol("BTC/USD")
		decision := NewDecision(ActionEnter, "BTC/USD")
		symbol.Decisions.Push(*decision)
		symbol.Graphs.Push(NewGraph(time.Time{}))
		symbol.Categories.Push([]Category{{Symbol: "BTC/USD"}})
		symbol.Phase.Push(PhaseReading{})
		symbol.Cognition.Push(Cognition{Symbol: "BTC/USD"})
		symbol.Causal.Push(map[string]any{"precision": 0.5})

		state, err := thesis.MarshalState()
		var checkpoint map[string]any
		unmarshalErr := json.Unmarshal(state, &checkpoint)

		Convey("It should materialize durable thesis state", func() {
			So(err, ShouldBeNil)
			So(unmarshalErr, ShouldBeNil)
			So(checkpoint["tick"], ShouldEqual, float64(7))
			So(checkpoint["status"], ShouldEqual, "ready")
			symbols := checkpoint["symbols"].(map[string]any)
			bitcoin := symbols["BTC/USD"].(map[string]any)
			So(bitcoin["causal"].([]any), ShouldHaveLength, 1)
			So(checkpoint["equity"], ShouldNotBeNil)
		})
	})
}

func TestThesisSymbol(t *testing.T) {
	Convey("Given an empty thesis", t, func() {
		thesis := NewThesis(t.Context(), nil)

		first := thesis.Symbol("BTC/USD")
		second := thesis.Symbol("BTC/USD")

		Convey("It should return the canonical symbol state", func() {
			So(first, ShouldEqual, second)
			So(first.Symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func TestThesisForSymbol(t *testing.T) {
	Convey("Given a thesis containing independent symbol state", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")
		thesis.Symbol("ETH/USD")

		scoped, err := thesis.ForSymbol("BTC/USD")

		Convey("It should share only the selected symbol with incremental solvers", func() {
			So(err, ShouldBeNil)
			selected, found := scoped.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(selected, ShouldEqual, bitcoin)
			_, found = scoped.Symbols.Load("ETH/USD")
			So(found, ShouldBeFalse)
			So(scoped.CrossSection, ShouldEqual, thesis.CrossSection)
		})

		Convey("It should reject a symbol absent from the parent thesis", func() {
			_, err = thesis.ForSymbol("MISSING/USD")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestThesisForSymbols(t *testing.T) {
	Convey("Given a thesis containing independent symbol state", t, func() {
		thesis := NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")
		ether := thesis.Symbol("ETH/USD")
		thesis.Symbol("SOL/USD")

		scoped, err := thesis.ForSymbols([]string{"ETH/USD", "BTC/USD"})

		Convey("It should share exactly the requested symbols", func() {
			So(err, ShouldBeNil)
			selected, found := scoped.Symbols.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(selected, ShouldEqual, bitcoin)
			selected, found = scoped.Symbols.Load("ETH/USD")
			So(found, ShouldBeTrue)
			So(selected, ShouldEqual, ether)
			_, found = scoped.Symbols.Load("SOL/USD")
			So(found, ShouldBeFalse)
		})

		Convey("It should reject an empty scope and a missing name", func() {
			_, err = thesis.ForSymbols(nil)
			So(err, ShouldNotBeNil)
			_, err = thesis.ForSymbols([]string{"BTC/USD", "MISSING/USD"})
			So(err, ShouldNotBeNil)
		})
	})
}

func TestThesisStoreManifold(t *testing.T) {
	Convey("Given a fluid reading published to a thesis", t, func() {
		thesis := NewThesis(t.Context(), nil)
		reading := pmanifold.Reading{GuidanceSpeed: 0.75}

		thesis.StoreManifold(reading)
		stored, found := thesis.ManifoldSnapshot()

		Convey("It should expose one immutable atomic snapshot", func() {
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, reading)
		})

		Convey("It should preserve that snapshot in a symbol scope", func() {
			thesis.Symbol("BTC/USD")
			scoped, err := thesis.ForSymbol("BTC/USD")
			scopedReading, scopedFound := scoped.ManifoldSnapshot()

			So(err, ShouldBeNil)
			So(scopedFound, ShouldBeTrue)
			So(scopedReading, ShouldResemble, reading)
		})
	})
}

func TestThesisStorePhase(t *testing.T) {
	Convey("Given a universe phase sweep published to a thesis", t, func() {
		thesis := NewThesis(t.Context(), nil)
		reading := PhaseReading{Ready: true, Reason: "retaining history"}

		thesis.StorePhase(reading)
		stored, found := thesis.PhaseSnapshot()

		Convey("It should expose one immutable atomic snapshot", func() {
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, reading)
		})

		Convey("It should preserve that snapshot in a symbol scope", func() {
			thesis.Symbol("BTC/USD")
			scoped, err := thesis.ForSymbol("BTC/USD")
			scopedReading, scopedFound := scoped.PhaseSnapshot()

			So(err, ShouldBeNil)
			So(scopedFound, ShouldBeTrue)
			So(scopedReading, ShouldResemble, reading)
		})
	})
}

func TestThesisNewThesis(t *testing.T) {
	Convey("Given a thesis without market evidence", t, func() {
		thesis := NewThesis(t.Context(), nil)

		Convey("Its event clock should remain unset", func() {
			So(thesis.At.IsZero(), ShouldBeTrue)
		})
	})
}

func TestThesisAppendEquity(t *testing.T) {
	Convey("Given positive account equity", t, func() {
		thesis := NewThesis(t.Context(), nil)

		err := thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		})
		equity, found := thesis.Equity()

		Convey("It should retain the latest account summary", func() {
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(equity.Equity.Float64(), ShouldEqual, 200.0)
		})
	})

	Convey("Given an incomplete account valuation", t, func() {
		thesis := NewThesis(t.Context(), nil)

		err := thesis.AppendEquity(kraken.TradeBalanceResult{})

		Convey("It should reject the observation without dereferencing missing equity", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestThesisEquitySnapshot(t *testing.T) {
	Convey("Given successive complete account valuations", t, func() {
		thesis := NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		_, firstRevision, firstFound := thesis.EquitySnapshot()
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(201),
		}), ShouldBeNil)
		equity, secondRevision, secondFound := thesis.EquitySnapshot()

		Convey("It should identify each broker observation exactly once", func() {
			So(firstFound, ShouldBeTrue)
			So(secondFound, ShouldBeTrue)
			So(secondRevision, ShouldEqual, firstRevision+1)
			So(equity.Equity.Float64(), ShouldEqual, 201.0)
		})
	})
}

func BenchmarkThesisStoreManifold(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	reading := pmanifold.Reading{GuidanceSpeed: 0.75}

	for b.Loop() {
		thesis.StoreManifold(reading)
	}
}

func BenchmarkThesisManifoldSnapshot(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	thesis.StoreManifold(pmanifold.Reading{GuidanceSpeed: 0.75})

	for b.Loop() {
		_, _ = thesis.ManifoldSnapshot()
	}
}

func BenchmarkThesisStorePhase(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	reading := PhaseReading{Ready: true}

	for b.Loop() {
		thesis.StorePhase(reading)
	}
}

func BenchmarkThesisPhaseSnapshot(b *testing.B) {
	thesis := NewThesis(b.Context(), nil)
	thesis.StorePhase(PhaseReading{Ready: true})

	for b.Loop() {
		_, _ = thesis.PhaseSnapshot()
	}
}
