package manifold

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
bookSolver builds a solver reading a test book source. Advance is driven
directly rather than through the run loop, so a test observes exactly one field
advance instead of racing a goroutine for the same book state.
*/
func bookSolver(ctx context.Context) (*Solver, *testBooks) {
	books := newTestBooks()
	solver := NewSolver(ctx)
	solver.SetBooks(books)

	return solver, books
}

/* semaphore is the Level3 envelope as the venue now sends it: no orders. */
func semaphore(symbol string) *types.Envelope {
	envelope := types.NewEnvelope(types.EnvelopeLevel3)
	envelope.Level3Data.Symbol = symbol
	envelope.Level3Data.Type = "update"

	return envelope
}

func TestSolverStep(t *testing.T) {
	Convey("Given a Manifold Solver reading the venue book", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		solver, books := bookSolver(ctx)
		So(solver, ShouldNotBeNil)

		Convey("An advance projects every resting order the book holds", func() {
			books.rest("SYM/USD", spotbook.Bid, "b1", 10.0, 1.0)
			books.rest("SYM/USD", spotbook.Bid, "b2", 9.9, 2.0)
			books.rest("SYM/USD", spotbook.Ask, "a1", 10.1, 1.5)

			So(solver.Step(semaphore("SYM/USD")), ShouldNotBeNil)

			reading := solver.Advance()

			So(reading, ShouldNotBeNil)
			So(reading.State.N, ShouldEqual, 3)
			So(reading.Pos, ShouldBeNil)
			So(reading.MomRho, ShouldBeNil)

			snapshot := solver.Snapshot()
			So(snapshot, ShouldNotBeNil)
			So(snapshot.State.N, ShouldEqual, 3)
			So(snapshot.Pos, ShouldNotBeEmpty)
			So(snapshot.MomRho, ShouldNotBeEmpty)
		})

		Convey("A non-Level3 envelope is a no-op", func() {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			result := solver.Step(envelope)

			So(result, ShouldNotBeNil)
			So(result.Manifold, ShouldBeNil)
		})

		Convey("A one-sided book yields ready particles without a center", func() {
			books.rest("SYM/USD", spotbook.Bid, "b1", 10.0, 1.0)

			So(solver.Step(semaphore("SYM/USD")), ShouldNotBeNil)

			reading := solver.Advance()

			So(reading, ShouldNotBeNil)
			So(reading.State.N, ShouldEqual, 1)
		})

		Convey("The domain spans every symbol the venue books", func() {
			books.rest("FIRST/USD", spotbook.Bid, "first", 10, 1)

			first := solver.Advance()

			So(first, ShouldNotBeNil)
			So(first.State.N, ShouldEqual, 1)

			books.rest("SECOND/USD", spotbook.Bid, "second-1", 20, 1)
			books.rest("SECOND/USD", spotbook.Bid, "second-2", 19, 1)

			second := solver.Advance()

			// One domain holds the whole universe: the second symbol's orders
			// join the first symbol's rather than replacing them, and each
			// advance owns its own immutable reading of the population.
			So(second, ShouldNotBeNil)
			So(first, ShouldNotEqual, second)
			So(second.State.N, ShouldEqual, 3)
		})

		Convey("An order pulled from the book leaves the domain", func() {
			books.rest("BOOK/USD", spotbook.Bid, "bid-1", 10, 1)
			books.rest("BOOK/USD", spotbook.Bid, "bid-2", 9, 1)

			So(solver.Advance().State.N, ShouldEqual, 2)

			books.pull("BOOK/USD", spotbook.Bid, "bid-1", 10)

			reading := solver.Advance()

			So(reading, ShouldNotBeNil)
			So(reading.State.N, ShouldEqual, 1)
			So(solver.Error(), ShouldBeNil)
		})

		Convey("An order that never loaded is not evicted when it vanishes", func() {
			// The book is the only residency authority, so an order that
			// arrived and left between two advances was never a particle. Its
			// absence must not become a departure the domain would reject.
			books.rest("TRANSIENT/USD", spotbook.Bid, "keep", 10, 1)
			books.rest("TRANSIENT/USD", spotbook.Bid, "gone", 9, 1)
			books.pull("TRANSIENT/USD", spotbook.Bid, "gone", 9)

			reading := solver.Advance()

			So(solver.Error(), ShouldBeNil)
			So(reading, ShouldNotBeNil)
			So(reading.State.N, ShouldEqual, 1)
		})

		Convey("Repeated advances over a churning book stay resident-exact", func() {
			for round := 0; round < 16; round++ {
				books.rest("CHURN/USD", spotbook.Bid, "steady", 10, 1)
				books.rest("CHURN/USD", spotbook.Ask, "transient", 10.5, 1)

				So(solver.Advance(), ShouldNotBeNil)
				So(solver.Error(), ShouldBeNil)

				books.pull("CHURN/USD", spotbook.Ask, "transient", 10.5)

				So(solver.Advance(), ShouldNotBeNil)
				So(solver.Error(), ShouldBeNil)
			}

			state := solver.physics.State()
			So(state.N, ShouldEqual, 1)

			for index := 0; index < state.N; index++ {
				So(math.IsNaN(float64(state.Energy[index])), ShouldBeFalse)
				So(math.IsInf(float64(state.Energy[index]), 0), ShouldBeFalse)
				So(math.IsNaN(float64(state.Mass[index])), ShouldBeFalse)
				So(math.IsInf(float64(state.Mass[index]), 0), ShouldBeFalse)
			}
		})

		Convey("An advance with no book is not an advance", func() {
			bare := NewSolver(ctx)
			defer bare.Close()

			So(bare.Advance(), ShouldBeNil)
			So(bare.Error(), ShouldBeNil)
		})
	})
}

func TestSolverAdvance(t *testing.T) {
	Convey("Given a field whose producer advances independently of delivery", t, func() {
		solver, books := bookSolver(context.Background())
		defer solver.Close()
		So(solver.Reading(), ShouldBeNil)
		So(solver.Snapshot(), ShouldBeNil)
		books.rest("TEST/USD", spotbook.Bid, "bid", 100, 1)
		books.rest("TEST/USD", spotbook.Ask, "ask", 101, 1)
		before := time.Now()
		first := solver.Advance()
		So(first, ShouldNotBeNil)
		So(first.At.Before(before), ShouldBeFalse)
		So(first.Version, ShouldEqual, 1)

		Convey("later deliveries carry the same immutable producer identity", func() {
			for range 3 {
				envelope := solver.Step(semaphore("TEST/USD"))
				So(envelope.Manifold, ShouldEqual, first)
				So(envelope.Manifold.At, ShouldResemble, first.At)
				So(envelope.Manifold.Version, ShouldEqual, first.Version)
			}
		})

		Convey("the viewer snapshot has the identity of its resident state", func() {
			snapshot := solver.Snapshot()
			So(snapshot.At, ShouldResemble, first.At)
			So(snapshot.Version, ShouldEqual, first.Version)
		})

		Convey("a changed book produces a new identity without modifying the old reading", func() {
			books.pull("TEST/USD", spotbook.Ask, "ask", 101)
			second := solver.Advance()
			So(second, ShouldNotBeNil)
			So(second.Version, ShouldEqual, first.Version+1)
			So(second.At.Before(first.At), ShouldBeFalse)
			So(first.Version, ShouldEqual, 1)
			So(first.N, ShouldEqual, 2)
			So(second.N, ShouldEqual, 1)
			So(solver.Reading(), ShouldEqual, second)
		})
	})
}

func BenchmarkSolverAdvance(b *testing.B) {
	// A two-sided fixture with 32 levels per side exercises real resident physics.
	solver, books := bookSolver(context.Background())
	defer solver.Close()

	for level := range 32 {
		books.rest("TEST/USD", spotbook.Bid, fmt.Sprint("bid", level), 100-float64(level), 1)
		books.rest("TEST/USD", spotbook.Ask, fmt.Sprint("ask", level), 101+float64(level), 1)
	}
	b.ReportAllocs()

	for b.Loop() {
		if solver.Advance() == nil {
			b.Fatalf("resident advance failed: %v", solver.Error())
		}
	}
}
