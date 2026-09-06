package strategy

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestAllocationReceiptReport(t *testing.T) {
	Convey("Venue callbacks publish immutable monotonic facts without accessing a model", t, func() {
		receipt := &AllocationReceipt{}
		at := time.Unix(100, 0)
		Convey("An aborted decision cannot be resurrected by a delayed submission callback", func() {
			receipt.Report(hindsight.AllocationResult{State: "aborted", At: at, Detail: "repricing failed"})
			receipt.Report(hindsight.AllocationResult{State: "submitted", At: at.Add(time.Second)})
			So(receipt.Result.Load().State, ShouldEqual, "aborted")
		})
		Convey("A partial fill remains realized if the unfilled remainder is cancelled", func() {
			receipt.Report(hindsight.AllocationResult{State: "filled", At: at})
			receipt.Report(hindsight.AllocationResult{State: "aborted", At: at.Add(time.Second)})
			So(receipt.Result.Load().State, ShouldEqual, "filled")
		})
		Convey("Concurrent duplicate callbacks and observation retain a single immutable result", func() {
			var workers sync.WaitGroup
			for range 8 {
				workers.Go(func() {
					receipt.Report(hindsight.AllocationResult{State: "submitted", At: at})
					receipt.Result.Load()
				})
			}
			workers.Wait()
			So(receipt.Result.Load().State, ShouldEqual, "submitted")
		})
	})
}

func TestAllocationReceiptObserve(t *testing.T) {
	Convey("Submission alone does not prove an allocation filled", t, func() {
		receipt := &AllocationReceipt{}
		event := hindsight.LifecycleEvent{At: time.Unix(100, 0), Kind: "execution_submitted", Execution: &hindsight.ExecutionFact{}}
		receipt.Observe(event)
		So(receipt.Result.Load().State, ShouldEqual, "submitted")
		Convey("An unfilled terminal order aborts", func() {
			event.Kind = "execution_terminal"
			event.Execution.OrderStatus = "canceled"
			receipt.Observe(event)
			So(receipt.Result.Load().State, ShouldEqual, "aborted")
		})
		Convey("A failed remainder still carries the confirmed cumulative fill", func() {
			event.Kind = "execution_failed"
			event.Execution.CumQty = "0.5"
			receipt.Observe(event)
			So(receipt.Result.Load().State, ShouldEqual, "filled")
		})
		Convey("Malformed fill quantities fail visibly", func() {
			event.Kind = "entry_fill"
			event.Execution.CumQty = "invalid"
			So(func() { receipt.Observe(event) }, ShouldPanic)
		})
		Convey("A cumulative partial fill establishes the allocation", func() {
			event.Kind = "increase_fill"
			event.Execution.CumQty = "0.5"
			receipt.Observe(event)
			So(receipt.Result.Load().State, ShouldEqual, "filled")
		})
	})
}
