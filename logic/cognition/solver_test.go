package cognition

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSolverStep(t *testing.T) {
	Convey("Given a Cognition Solver", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		solver := NewSolver(ctx)
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

						envelope := types.NewEnvelope(types.EnvelopeUnknown)
						envelope.Categories = []types.Category{
							{
								At:         time.Now(),
								Symbol:     sym,
								Type:       categoryType,
								Confidence: 0.95,
								Strength:   0.9,
								Maturity:   1.0,
							},
						}

						_ = solver.Step(envelope)
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
		solver := NewSolver(t.Context(), WithMaxSequenceLength(1))
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
