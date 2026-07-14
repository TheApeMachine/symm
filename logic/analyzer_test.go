package logic

import (
	"context"
	"encoding/json"
	"iter"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
readyStageGate admits every analyzer stage in focused synchronous tests.
*/
type readyStageGate struct{}

/*
Ready reports that the focused analyzer fixture has completed preflight.
*/
func (readyStageGate) Ready(system.StageType) bool {
	return true
}

/*
staticLevel3Source exposes deterministic SDK book managers to focused logic tests.
*/
type staticLevel3Source struct {
	managers []*spot.BookManager
}

/*
Books yields every managed test book through the production Level3 boundary.
*/
func (source staticLevel3Source) Books() iter.Seq[*spot.BookManager] {
	return func(yield func(*spot.BookManager) bool) {
		for _, manager := range source.managers {
			if !yield(manager) {
				return
			}
		}
	}
}

func TestAnalyzerUpdate(t *testing.T) {
	Convey("Given measurements for two symbols", t, func() {
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements,
			&types.Measurement{
				Stream: types.Hawkes, Metric: types.MetricArrivalRate,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
			},
			&types.Measurement{
				Stream: types.PumpDump, Metric: types.MetricRVOL,
				Symbol: "ETH/USD", At: time.Unix(2, 0),
			},
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("It should write one symbol-local graph directly onto the thesis", func() {
			So(thesis.Graphs, ShouldHaveLength, 2)
			So(thesis.Graphs["BTC/USD"].Nodes().Len(), ShouldEqual, 1)
			So(thesis.Graphs["ETH/USD"].Nodes().Len(), ShouldEqual, 1)
		})
	})
}

func TestAnalyzerUpdateComposesRelationships(t *testing.T) {
	Convey("Given comparable typed measurements on one thesis", t, func() {
		thesis := types.NewThesis(nil)
		positive := 0.5
		negative := -0.5
		thesis.Measurements = append(thesis.Measurements,
			&types.Measurement{
				Source: types.SourceHawkes, Stream: types.Hawkes,
				Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
				Unit: types.UnitDimensionless, Normalized: &positive,
				Validity: types.MeasurementValidity{State: types.ValidityValid},
			},
			&types.Measurement{
				Source: types.SourcePumpDump, Stream: types.PumpDump,
				Metric: types.MetricStrength, Subject: types.SubjectTradeArrivals,
				Symbol: "BTC/USD", At: time.Unix(1, 0),
				Unit: types.UnitDimensionless, Normalized: &negative,
				Validity: types.MeasurementValidity{State: types.ValidityValid},
			},
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("It should retain the contradiction on the symbol graph", func() {
			evidenceGraph := thesis.Graphs["BTC/USD"]
			edges := evidenceGraph.Edges()
			So(edges.Next(), ShouldBeTrue)
			edge := edges.Edge()
			lines := evidenceGraph.Lines(edge.From().ID(), edge.To().ID())
			So(lines.Next(), ShouldBeTrue)
			So(lines.Line().(*types.Edge).Type, ShouldEqual, types.Contradicts)
		})
	})
}

func TestAnalyzerUpdateLevel3(t *testing.T) {
	Convey("Given an authoritative SDK-managed Level3 book", t, func() {
		viper.Set("market.l3_depth", 10)
		viper.Set("market.forecast.rls.initial_variance", 1.0)
		viper.Set("market.forecast.rls.forgetting_factor", 1.0)
		viper.Set("market.manifold.lifetime_capacity", 256)
		manager := spot.NewBookManager()
		managed := manager.CreateBook("BTC/USD", 10)
		at := time.Unix(1, 0)
		managed.Update(&book.UpdateOptions{
			Direction: book.Bid, ID: "bid-1",
			Price: decimal.NewFromFloat64(99.5), Quantity: decimal.NewFromFloat64(2),
			Timestamp: at,
		})
		managed.Update(&book.UpdateOptions{
			Direction: book.Ask, ID: "ask-1",
			Price: decimal.NewFromFloat64(100.5), Quantity: decimal.NewFromFloat64(3),
			Timestamp: at,
		})
		uiHub := make(chan []byte, 4)
		analyzer := NewAnalyzer(
			context.Background(),
			readyStageGate{},
			staticLevel3Source{managers: []*spot.BookManager{manager}},
		)
		defer analyzer.Close()
		thesis := types.NewThesis(uiHub)

		analyzer.Update(thesis)

		Convey("Then Level3 admits and consumes the book exactly once", func() {
			slot, exists := analyzer.level3.engine.Slot("BTC/USD")
			So(exists, ShouldBeTrue)
			So(slot.Advance().StateProduced, ShouldBeFalse)
			So(len(uiHub), ShouldEqual, 0)
			So(thesis.Manifold, ShouldHaveLength, 1)
			So(thesis.Manifold[0].(manifold.State).Epoch, ShouldEqual, uint64(1))

			analyzer.Update(thesis)
			So(thesis.Manifold, ShouldHaveLength, 1)

			managed.Update(&book.UpdateOptions{
				Direction: book.Bid, ID: "bid-1",
				Price: decimal.NewFromFloat64(99.5), Quantity: decimal.NewFromFloat64(4),
				Timestamp: at.Add(time.Second),
			})
			analyzer.Update(thesis)
			So(thesis.Manifold, ShouldHaveLength, 2)
			So(thesis.Manifold[1].(manifold.State).Epoch, ShouldEqual, uint64(2))

			thesis.Publish()
			So(len(uiHub), ShouldEqual, 1)

			var frame struct {
				Manifold []manifold.State `json:"manifold"`
			}

			So(json.Unmarshal(<-uiHub, &frame), ShouldBeNil)
			So(frame.Manifold, ShouldHaveLength, 2)
		})
	})
}

func BenchmarkAnalyzerUpdate(b *testing.B) {
	analyzer := &Analyzer{}

	for b.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements, &types.Measurement{
			Stream: types.Hawkes, Metric: types.MetricArrivalRate,
			Symbol: "BTC/USD", At: time.Unix(1, 0),
		})
		analyzer.Update(thesis)
	}
}
