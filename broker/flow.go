package broker

import "github.com/theapemachine/datura"

type FlowStats struct {
	SubmittedCount         int
	OpenOrderCount         int
	PreflightRejectedCount int
	FilledCount            int
}

func (desk *Desk) recordPrivateSubmission(order *datura.Artifact) {
	if desk == nil || order == nil {
		return
	}
	if datura.Peek[string](order, "method") == "add_order" {
		desk.submittedCount.Add(1)
	}
}

func (desk *Desk) recordExecutionFlow(artifact *datura.Artifact, status string) {
	if desk == nil {
		return
	}

	switch status {
	case "filled":
		desk.filledCount.Add(1)
	case "rejected":
		if executionRejectReason(artifact) == "" {
			return
		}
		desk.preflightRejectedCount.Add(1)
		desk.publishPreflightRejectDiagnostic(artifact)
	}
}

func executionRejectReason(artifact *datura.Artifact) string {
	if artifact == nil {
		return ""
	}
	if reason := datura.Peek[string](artifact, "reject_reason"); reason != "" {
		return reason
	}
	return datura.Peek[string](artifact, "data", 0, "reject_reason")
}

func (desk *Desk) publishPreflightRejectDiagnostic(artifact *datura.Artifact) {
	if desk == nil || artifact == nil {
		return
	}

	reason := datura.Peek[string](artifact, "data", 0, "reject_reason")
	if reason == "" {
		reason = executionRejectReason(artifact)
	}

	symbol := datura.Peek[string](artifact, "data", 0, "symbol")
	if symbol == "" {
		symbol = datura.Peek[string](artifact, "scope")
	}

	desk.publishDiagnosticPayload(datura.Map[any]{
		"severity":               "warning",
		"symbol":                 symbol,
		"side":                   datura.Peek[string](artifact, "data", 0, "side"),
		"order_type":             datura.Peek[string](artifact, "data", 0, "order_type"),
		"reason":                 reason,
		"reject_reason":          reason,
		"decision_id":            datura.Peek[string](artifact, "data", 0, "decision_id"),
		"action_id":              datura.Peek[string](artifact, "data", 0, "action_id"),
		"cl_ord_id":              datura.Peek[string](artifact, "data", 0, "cl_ord_id"),
		"quote_age":              datura.Peek[float64](artifact, "data", 0, "quote_age"),
		"spread_bps":             datura.Peek[float64](artifact, "data", 0, "spread_bps"),
		"projected_slippage_bps": datura.Peek[float64](artifact, "data", 0, "projected_slippage_bps"),
		"depth_coverage":         datura.Peek[float64](artifact, "data", 0, "depth_coverage"),
		"pending_count":          desk.pendingCount(),
	})
}

func (desk *Desk) FlowStats() FlowStats {
	if desk == nil {
		return FlowStats{}
	}

	return FlowStats{
		SubmittedCount:         int(desk.submittedCount.Swap(0)),
		OpenOrderCount:         desk.PendingEntryCount(),
		PreflightRejectedCount: int(desk.preflightRejectedCount.Swap(0)),
		FilledCount:            int(desk.filledCount.Swap(0)),
	}
}
