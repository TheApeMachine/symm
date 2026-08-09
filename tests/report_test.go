package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketReport(t *testing.T) {
	Convey("Given a scheduled market with a dropped frame", t, func() {
		symbol := testtypes.NewSymbol("SIM1/USD", 100, 81)
		config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
		config.Schedule = []testtypes.RegimeTransition{{
			Tick: 0, Symbol: symbol.Pair, State: testtypes.SidewaysChop,
		}}
		config.Faults.Rules = []testtypes.FaultRule{{
			Channel: "ticker", Occurrence: 1, Action: testtypes.FaultDrop,
		}}
		market, err := NewMarketWithScenario(context.Background(), config)
		So(err, ShouldBeNil)
		defer market.Close()

		market.Tick()
		report := market.Report()

		Convey("The report should separate oracle state from transport mechanics", func() {
			So(report.Tick, ShouldEqual, uint64(1))
			So(report.Timeline, ShouldHaveLength, 1)
			So(report.Timeline[0].State, ShouldEqual, testtypes.SidewaysChop)
			So(report.RegimeExposure[symbol.Pair][testtypes.SidewaysChop],
				ShouldEqual, uint64(1))
			So(report.PublicTransport.Dropped, ShouldEqual, 1)
			So(report.PublicTransport.Frames, ShouldNotBeEmpty)
		})
	})
}

func TestMarketValidate(t *testing.T) {
	Convey("Given a market without execution state", t, func() {
		market := NewMarket(context.Background(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100, 82),
		})
		defer market.Close()

		Convey("Its simulator-owned invariants should hold", func() {
			So(market.Validate(), ShouldBeNil)
		})
	})
}

func TestMarketWriteArtifact(t *testing.T) {
	Convey("Given a completed deterministic market tick", t, func() {
		market := NewMarket(context.Background(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100, 83),
		})
		defer market.Close()
		market.Tick()
		path := filepath.Join(t.TempDir(), "scenario.json")

		err := market.WriteArtifact(path)
		So(err, ShouldBeNil)
		payload, err := os.ReadFile(path)
		So(err, ShouldBeNil)
		report := SimulatorReport{}
		err = json.Unmarshal(payload, &report)

		Convey("The artifact should contain replay identity and generated frames", func() {
			So(err, ShouldBeNil)
			So(report.Scenario.Seed, ShouldEqual, market.Config.Seed)
			So(report.Tick, ShouldEqual, uint64(1))
			So(report.PublicTransport.Frames, ShouldNotBeEmpty)
		})
	})
}

func BenchmarkMarketReport(b *testing.B) {
	market := NewMarket(context.Background(), []*testtypes.Symbol{
		testtypes.NewSymbol("REPORT/USD", 100, 84),
	})
	defer market.Close()
	market.Tick()

	for b.Loop() {
		if _, err := json.Marshal(market.Report()); err != nil {
			b.Fatal(err)
		}
	}
}
