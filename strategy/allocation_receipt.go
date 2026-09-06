package strategy

import (
	"math/big"
	"sync/atomic"

	"github.com/theapemachine/symm/hindsight"
)

/*
AllocationReceipt carries execution facts from venue workers to the serialized
capital owner. A filled allocation remains realized after a partial cancellation;
an aborted decision cannot be resurrected by a late callback.
*/
type AllocationReceipt struct {
	Result atomic.Pointer[hindsight.AllocationResult]
}

/* Report advances execution state without mutating a learner from the venue goroutine. */
func (receipt *AllocationReceipt) Report(result hindsight.AllocationResult) {
	if receipt == nil {
		return
	}

	if result.At.IsZero() || (result.State != "submitted" && result.State != "filled" && result.State != "aborted") {
		panic("allocation: execution state and producer time required")
	}

	for {
		previous := receipt.Result.Load()

		if previous != nil && (previous.State == "filled" || previous.State == "aborted" || previous.State == result.State) {
			return
		}

		if receipt.Result.CompareAndSwap(previous, &result) {
			return
		}
	}
}

/* Observe distinguishes a venue acknowledgement from an actually filled allocation. */
func (receipt *AllocationReceipt) Observe(event hindsight.LifecycleEvent) {
	if receipt == nil || event.Execution == nil {
		return
	}
	result := hindsight.AllocationResult{At: event.At}
	switch event.Kind {
	case "execution_submitted":
		result.State = "submitted"
	case "execution_refused":
		result.State, result.Detail = "aborted", event.Kind
	case "entry_fill", "increase_fill", "execution_terminal", "execution_failed":
		quantity, valid := new(big.Rat).SetString(event.Execution.CumQty)

		if event.Execution.CumQty != "" && (!valid || quantity.Sign() < 0) {
			panic("allocation: invalid cumulative executed quantity")
		}

		if valid && quantity.Sign() > 0 {
			result.State = "filled"
		}

		if result.State == "" && (event.Kind == "execution_terminal" || event.Kind == "execution_failed") {
			result.State, result.Detail = "aborted", "order ended without a confirmed fill"
		}

		if result.State == "" {
			panic("allocation: fill event requires positive cumulative quantity")
		}
	}

	if result.State != "" {
		receipt.Report(result)
	}
}
