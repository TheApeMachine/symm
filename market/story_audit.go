package market

import (
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func playbookAuditTrace(trace reasoning.ReasonTrace) []map[string]any {
	if len(trace.Nodes) == 0 {
		return nil
	}

	nodes := make([]map[string]any, 0, len(trace.Nodes))

	for _, node := range trace.Nodes {
		nodes = append(nodes, map[string]any{
			"key":       node.Key,
			"depth":     node.Depth,
			"reachable": node.Reachable,
			"fires":     node.Fires,
			"latched":   node.Latched,
			"fired":     node.Fired,
			"leaves":    node.Leaves,
		})
	}

	return nodes
}

func playbookAuditMeasurement(measurement types.Measurement) map[string]any {
	fields := map[string]any{
		"spread_bps": measurement.SpreadBPS,
		"volume":     measurement.Volume,
		"confidence": measurement.Confidence,
	}

	if measurement.HasBookDepth() {
		fields["book_bids"] = len(measurement.BookBids)
		fields["book_asks"] = len(measurement.BookAsks)
	}

	return fields
}
