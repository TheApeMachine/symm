package types

import (
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
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
		symbol.Decisions.Store("BTC/USD", decision)
		symbol.Graphs.Store("market_graph", map[string]any{"ready": true})
		symbol.Categories.Store("BTC/USD", []Category{{Symbol: "BTC/USD"}})
		symbol.Phase.Store("BTC/USD", PhaseReading{Symbol: "BTC/USD"})
		symbol.Cognition.Store("BTC/USD", Cognition{Symbol: "BTC/USD"})
		symbol.Causal.Store("BTC/USD", map[string]any{"precision": 0.5})

		state, err := thesis.MarshalState()
		var checkpoint map[string]any
		unmarshalErr := json.Unmarshal(state, &checkpoint)

		Convey("It should materialize durable thesis state", func() {
			So(err, ShouldBeNil)
			So(unmarshalErr, ShouldBeNil)
			So(checkpoint["tick"], ShouldEqual, float64(7))
			So(checkpoint["status"], ShouldEqual, "ready")
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
