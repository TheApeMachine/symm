package cognition

import (
	"context"
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
