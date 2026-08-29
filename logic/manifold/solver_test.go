package manifold

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestSolverStep(t *testing.T) {
	Convey("Given a Manifold Solver", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		thesis := types.NewThesis(ctx)

		solver := NewSolver(ctx)
		So(solver, ShouldNotBeNil)

		Convey("When concurrent Hawkes measurements arrive across multiple symbols", func() {
			var waitGroup sync.WaitGroup
			symbolCount := 16

			for symbolIndex := 0; symbolIndex < symbolCount; symbolIndex++ {
				symbol := fmt.Sprintf("SYM%d/USD", symbolIndex)
				_ = thesis.Symbol(symbol)

				waitGroup.Add(1)
				go func(sym string) {
					defer waitGroup.Done()

					envelope := types.NewEnvelope(types.EnvelopeTrade)
					envelope.Hawkes = &data.Measurement[float64]{
						Source: "hawkes",
						Label:  sym,
						At:     time.Now(),
					}
					_ = solver.Step(envelope)
				}(symbol)
			}

			waitGroup.Wait()

			Convey("Execution should complete cleanly without Metal command buffer assertions", func() {
				So(solver.Name(), ShouldEqual, "manifold")
			})
		})

		Convey("When envelope carries no Hawkes measurement", func() {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			result := solver.Step(envelope)

			So(result, ShouldNotBeNil)
			So(result.Manifold, ShouldBeNil)
		})
	})
}
