package cognition

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	nmsequence "github.com/theapemachine/symm/nomagique/data/sequence"

	"github.com/theapemachine/symm/nomagique/runtime"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestSolverNext(t *testing.T) {
	Convey("Registered category batches preserve strength, symbols and timestamps", t, func() {
		solver := NewSolver(t.Context())
		solver.Transition(runtime.READY)
		category := data.NewMeasurement[float64]("category", nil)
		category.Result = [][]types.Category{
			{
				{Symbol: "BTC/USD", At: time.Unix(10, 0), Type: types.Turbulent, Confidence: 1, Strength: 0.1},
				{Symbol: "BTC/USD", At: time.Unix(10, 0), Type: types.OrganicTrend, Confidence: 0.8, Strength: 1},
			},
			{{Symbol: "ETH/USD", At: time.Unix(11, 0), Type: types.Turbulent, Confidence: 1, Strength: 1}},
		}
		measurement := solver.Register()
		measurement.Peers = []*data.Measurement[float64]{category}
		result := nmsequence.Read[*data.Measurement[float64]](solver.Next(nmsequence.NewValue[*data.Measurement[float64]](measurement)))
		readings, ok := result.Result.([]types.Cognition)
		So(ok, ShouldBeTrue)
		So(len(readings), ShouldEqual, 2)
		So(readings[0].Symbol, ShouldEqual, "BTC/USD")
		So(readings[0].At, ShouldEqual, time.Unix(10, 0))
		So(readings[0].Sequence, ShouldContainSubstring, string(types.OrganicTrend))
		So(readings[1].Symbol, ShouldEqual, "ETH/USD")
		So(readings[1].At, ShouldEqual, time.Unix(11, 0))
		So(solver.Error(), ShouldBeNil)
	})

	Convey("Given a Cognition Solver", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		solver := NewSolver(ctx)
		solver.Transition(runtime.READY)
		So(solver, ShouldNotBeNil)

		Convey("When processing concurrent category batches across 64 symbols", func() {
			var waitGroup sync.WaitGroup
			symbolCount := 64
			batchCount := 20

			for symbolIndex := 0; symbolIndex < symbolCount; symbolIndex++ {
				symbol := fmt.Sprintf("SYM%d/USD", symbolIndex)

				waitGroup.Add(1)
				go func(sym string) {
					defer waitGroup.Done()

					for batchIndex := 0; batchIndex < batchCount; batchIndex++ {
						categoryType := types.CategoryTurbulent
						if batchIndex%2 == 0 {
							categoryType = types.CategoryOrganicTrend
						}

						m := data.NewMeasurement[float64]("cognition", nil)
						m.Label = sym
						cat := data.NewMeasurement[float64]("category", nil)
						cat.Label = sym
						cat.Result = [][]types.Category{{{
							Symbol: sym, At: time.Unix(int64(batchIndex+1), 0),
							Type: categoryType, Confidence: 0.95, Strength: 1,
						}}}
						m.Peers = []*data.Measurement[float64]{cat}

						_ = nmsequence.Read[*data.Measurement[float64]](solver.Next(nmsequence.NewValue[*data.Measurement[float64]](m)))
					}
				}(symbol)
			}

			waitGroup.Wait()

			Convey("All symbol states should be isolated without data races", func() {
				state := solver.getSymbolState("SYM0/USD")
				So(state, ShouldNotBeNil)
				So(state.hasReading, ShouldBeTrue)
			})
		})
	})
}

func TestSolverProcessBatch(t *testing.T) {
	Convey("Given category transitions committed out of event-time order", t, func() {
		solver := NewSolver(t.Context())
		solver.maxSeqLen = 1
		newer := []types.Category{{
			At: time.Unix(2, 0), Symbol: "TEST/USD",
			Type: types.OrganicTrend, Confidence: 1, Strength: 1,
		}}
		older := []types.Category{{
			At: time.Unix(1, 0), Symbol: "TEST/USD",
			Type: types.Turbulent, Confidence: 1, Strength: 1,
		}}

		So(solver.processBatch(
			"TEST/USD", newer, 0.5, map[string]types.Cognition{},
		), ShouldBeNil)
		So(solver.processBatch(
			"TEST/USD", older, 0.5, map[string]types.Cognition{},
		), ShouldBeNil)

		var episodeOrdinals []uint64
		solver.tree.WalkPrefix([]byte("e/"), func(key, _ []byte) bool {
			episodeOrdinals = append(
				episodeOrdinals,
				binary.BigEndian.Uint64(key[len("e/"):len("e/")+8]),
			)

			return true
		})

		Convey("DMT recency follows transition order rather than timestamps", func() {
			So(solver.tickCounter.Load(), ShouldEqual, uint64(2))
			So(episodeOrdinals, ShouldResemble, []uint64{2})
			So(solver.Error(), ShouldBeNil)
		})
	})
}

func TestSolverREMConsolidationAndCategoryCoverage(t *testing.T) {
	Convey("Given a Cognition Solver", t, func() {
		solver := NewSolver(t.Context())
		solver.maxSeqLen = 2

		Convey("When 128 transitions occur across non-legacy categories", func() {
			rows := map[string]types.Cognition{}
			categories := []types.CategoryType{
				types.VerticalIgnition,
				types.CoiledCompression,
				types.RiskOnSurge,
				types.LiquidityVacuum,
			}

			for tick := 0; tick < 128; tick++ {
				cat := categories[tick%len(categories)]
				err := solver.processBatch("BTC/USD", []types.Category{{
					At:         time.Unix(int64(tick+1), 0),
					Symbol:     "BTC/USD",
					Type:       cat,
					Confidence: 0.9,
					Strength:   0.8,
				}}, 0.5, rows)
				So(err, ShouldBeNil)
			}

			Convey("REM consolidation runs on the 128th tick", func() {
				reading, found := solver.Reading("BTC/USD")
				So(found, ShouldBeTrue)
				So(reading.REMConsolidating, ShouldBeTrue)
				So(solver.tickCounter.Load(), ShouldEqual, uint64(128))
				So(reading.Winner, ShouldNotBeBlank)
			})
		})
	})
}

func TestSolverStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Solver{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(nmsequence.Read[*data.Measurement[float64]](node.Next(nmsequence.NewValue[*data.Measurement[float64]](measurement))), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func BenchmarkSolverNext(b *testing.B) {
	solver := NewSolver(b.Context())
	solver.Transition(runtime.READY)
	measurement := solver.Register()
	category := data.NewMeasurement[float64]("category", nil)
	measurement.Peers = []*data.Measurement[float64]{category}
	categories := [][]types.Category{{{
		Symbol: "BTC/USD", Confidence: 0.8, Strength: 1,
	}}}
	category.Result = categories
	sequence := int64(0)
	b.ReportAllocs()

	for b.Loop() {
		sequence++
		categories[0][0].At = time.Unix(sequence, 0)
		categories[0][0].Type = []types.CategoryType{types.OrganicTrend, types.Turbulent}[sequence%2]
		nmsequence.Read[*data.Measurement[float64]](solver.Next(nmsequence.NewValue[*data.Measurement[float64]](measurement)))

		if err := solver.Error(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSolverStepEmptyBatch(t *testing.T) {
	Convey("An empty category publication produces no cognition observation", t, func() {
		solver := NewSolver(t.Context())
		solver.Transition(runtime.READY)
		category := data.NewMeasurement[float64]("category", nil)
		category.Result = [][]types.Category{}
		measurement := solver.Register()
		measurement.Peers = []*data.Measurement[float64]{category}
		So(nmsequence.Read[*data.Measurement[float64]](solver.Next(nmsequence.NewValue[*data.Measurement[float64]](measurement))), ShouldBeNil)
		So(solver.Error(), ShouldBeNil)
	})
}
