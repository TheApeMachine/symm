package manifold

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestSolverStep(t *testing.T) {
	Convey("Given a Manifold Solver", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		solver := NewSolver(ctx)
		So(solver, ShouldNotBeNil)

		Convey("Stepping a Level3 envelope projects its orders and advances once", func() {
			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = kraken.Level3Data{
				Symbol:    "SYM/USD",
				Timestamp: time.Now(),
				Bids: []kraken.Level3Order{
					{Event: "add", OrderID: "b1", LimitPrice: decimalPtr(10.0), OrderQty: decimalPtr(1.0), Timestamp: time.Now()},
					{Event: "add", OrderID: "b2", LimitPrice: decimalPtr(9.9), OrderQty: decimalPtr(2.0), Timestamp: time.Now()},
				},
				Asks: []kraken.Level3Order{
					{Event: "add", OrderID: "a1", LimitPrice: decimalPtr(10.1), OrderQty: decimalPtr(1.5), Timestamp: time.Now()},
				},
			}

			result := solver.Step(envelope)

			So(result, ShouldNotBeNil)
			So(result.Manifold, ShouldNotBeNil)
			So(result.Manifold.State.N, ShouldEqual, 3)
			So(result.Manifold.Pos, ShouldBeNil)
			So(result.Manifold.MomRho, ShouldBeNil)

			snapshot := solver.Snapshot()
			So(snapshot, ShouldNotBeNil)
			So(snapshot.State.N, ShouldEqual, 3)
			So(snapshot.Pos, ShouldNotBeEmpty)
			So(snapshot.MomRho, ShouldNotBeEmpty)
		})

		Convey("Stepping a non-Level3 envelope is a no-op", func() {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			result := solver.Step(envelope)

			So(result, ShouldNotBeNil)
			So(result.Manifold, ShouldBeNil)
		})

		Convey("A one-sided Level3 message yields ready particles without a center", func() {
			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = kraken.Level3Data{
				Symbol:    "SYM/USD",
				Timestamp: time.Now(),
				Bids: []kraken.Level3Order{
					{Event: "add", OrderID: "b1", LimitPrice: decimalPtr(10.0), OrderQty: decimalPtr(1.0), Timestamp: time.Now()},
				},
			}

			result := solver.Step(envelope)

			So(result, ShouldNotBeNil)
			So(result.Manifold, ShouldNotBeNil)
			So(result.Manifold.State.N, ShouldEqual, 1)
		})

		Convey("The resident domain accumulates across envelopes", func() {
			first := types.NewEnvelope(types.EnvelopeLevel3)
			first.Level3Data = kraken.Level3Data{
				Symbol: "FIRST/USD",
				Bids: []kraken.Level3Order{
					{Event: "add", OrderID: "first", LimitPrice: decimalPtr(10), OrderQty: decimalPtr(1)},
				},
			}
			solver.Step(first)

			second := types.NewEnvelope(types.EnvelopeLevel3)
			second.Level3Data = kraken.Level3Data{
				Symbol: "SECOND/USD",
				Bids: []kraken.Level3Order{
					{Event: "add", OrderID: "second-1", LimitPrice: decimalPtr(20), OrderQty: decimalPtr(1)},
					{Event: "add", OrderID: "second-2", LimitPrice: decimalPtr(19), OrderQty: decimalPtr(1)},
				},
			}
			solver.Step(second)

			// The domain is resident: the second message's two orders join the
			// first message's order rather than replacing it, and each envelope
			// still owns its own immutable reading of the population at its own
			// moment.
			So(first.Manifold, ShouldNotEqual, second.Manifold)
			So(first.Manifold.State.N, ShouldEqual, 1)
			So(second.Manifold.State.N, ShouldEqual, 3)
		})
	})
}

func decimalPtr(value float64) *decimal.Decimal {
	return decimal.NewFromFloat64(value)
}
