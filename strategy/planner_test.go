package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
crossSectionProbe mirrors how correlation, leadlag, sentiment, and liquidity
actually use Thesis.CrossSection: it observes one ticker row per tick and
records whether SymbolReturns can produce a genuine return yet. It isolates
Planner's CrossSection lifetime from any one signal's own scoring logic.
*/
type crossSectionProbe struct {
	rows         []kraken.TickerData
	returnCounts []int
}

func (probe *crossSectionProbe) Measure(thesis *types.Thesis) *types.Thesis {
	row := probe.rows[0]
	probe.rows = probe.rows[1:]

	thesis.CrossSection.ProcessUpdates([]kraken.TickerData{row})

	dst := make([]float64, thesis.CrossSection.MaxReturnWindow())
	written := thesis.CrossSection.SymbolReturns(row.Symbol, dst)
	probe.returnCounts = append(probe.returnCounts, written)

	return thesis
}

func TestPlannerUpdatePersistsCrossSectionAcrossTicks(t *testing.T) {
	Convey("Given a planner ticking twice for the same symbol", t, func() {
		start := time.Unix(1_700_000_000, 0)
		probe := &crossSectionProbe{
			rows: []kraken.TickerData{
				{Symbol: "BTC/USD", Last: decimal.NewFromFloat64(100), Timestamp: start},
				{Symbol: "BTC/USD", Last: decimal.NewFromFloat64(101), Timestamp: start.Add(time.Second)},
			},
		}

		planner := NewPlanner(context.Background(), nil, []types.Signal{probe}, nil)

		planner.Update()
		planner.Update()

		Convey("Then the second tick sees a return the first tick could not have", func() {
			So(probe.returnCounts, ShouldResemble, []int{0, 1})
		})
	})
}

func TestPlannerUpdateDedupesRepeatedTimestampsAcrossSignals(t *testing.T) {
	Convey("Given two signals observing the same ticker row in one tick", t, func() {
		at := time.Unix(1_700_000_000, 0)
		row := kraken.TickerData{Symbol: "BTC/USD", Last: decimal.NewFromFloat64(100), Timestamp: at}

		observer := func(thesis *types.Thesis) *types.Thesis {
			thesis.CrossSection.ProcessUpdates([]kraken.TickerData{row})
			return thesis
		}

		signals := []types.Signal{
			signalFunc(observer),
			signalFunc(observer),
		}

		planner := NewPlanner(context.Background(), nil, signals, nil)

		thesis := planner.Update()

		Convey("Then the shared cross-section keeps only one observation", func() {
			dst := make([]correlation.Sample, 1)
			So(thesis.CrossSection.SymbolSamples("BTC/USD", dst), ShouldEqual, 1)
		})
	})
}

type signalFunc func(*types.Thesis) *types.Thesis

func (fn signalFunc) Measure(thesis *types.Thesis) *types.Thesis {
	return fn(thesis)
}

func TestPlannerBeginCompleteTick(testingTB *testing.T) {
	Convey("Given a planner with no signals", testingTB, func() {
		planner := NewPlanner(context.Background(), nil, nil, nil)

		Convey("When a tick is begun and completed", func() {
			thesis := planner.BeginTick()
			result := planner.CompleteTick(thesis)

			Convey("Then it preserves the same thesis carrier", func() {
				So(result, ShouldEqual, thesis)
				So(result.CrossSection, ShouldEqual, planner.crossSection)
			})
		})
	})
}
