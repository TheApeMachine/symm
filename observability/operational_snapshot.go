package observability

import (
	"sort"
	"strings"
	"time"
)

func (metrics *OperationalMetrics) Snapshot() Snapshot {
	if metrics == nil {
		return Snapshot{}
	}

	metrics.mu.RLock()
	defer metrics.mu.RUnlock()

	return Snapshot{
		Bus:            metrics.busSnapshots(),
		WebSockets:     metrics.webSocketSnapshots(),
		MarketData:     metrics.marketDataSnapshots(),
		ExchangeErrors: metrics.exchangeErrorSnapshots(),
		Orders:         metrics.orderSnapshot(),
		Stops:          metrics.stopSnapshot(),
		Exposure:       metrics.exposure,
		Audit:          metrics.audit,
	}
}

func (metrics *OperationalMetrics) busStats(
	channel string,
	messageType string,
) *busChannelStats {
	key := channel + ":" + messageType
	stats := metrics.bus[key]

	if stats != nil {
		return stats
	}

	stats = &busChannelStats{
		snapshot: BusChannelSnapshot{
			Channel:     channel,
			MessageType: messageType,
		},
	}
	metrics.bus[key] = stats

	return stats
}

func (metrics *OperationalMetrics) webSocketStats(name string) *webSocketStats {
	stats := metrics.websockets[name]

	if stats != nil {
		return stats
	}

	stats = &webSocketStats{snapshot: WebSocketSnapshot{Name: name}}
	metrics.websockets[name] = stats

	return stats
}

func (metrics *OperationalMetrics) updateOrderCorrelation(
	clOrdID string,
	exchangeOrderID string,
	executionID string,
) {
	correlation := metrics.orders.correlations[clOrdID]
	correlation.ClOrdID = clOrdID

	if exchangeOrderID != "" {
		correlation.ExchangeOrderID = exchangeOrderID
	}

	if executionID != "" {
		correlation.ExecutionID = executionID
	}

	metrics.orders.correlations[clOrdID] = correlation
}

func (metrics *OperationalMetrics) recordOrderAck(
	clOrdID string,
	observedAt time.Time,
) {
	metrics.orders.snapshot.Acknowledged++

	if submittedAt := metrics.orders.submittedAt[clOrdID]; !submittedAt.IsZero() {
		metrics.orders.ackLatency += observedAt.Sub(submittedAt)
	}

	metrics.refreshOrderLatencyAverages()
}

func (metrics *OperationalMetrics) recordOrderFill(
	clOrdID string,
	observedAt time.Time,
) {
	metrics.orders.snapshot.Filled++

	if submittedAt := metrics.orders.submittedAt[clOrdID]; !submittedAt.IsZero() {
		metrics.orders.fillLatency += observedAt.Sub(submittedAt)
	}

	metrics.clearOrder(clOrdID)
	metrics.refreshOrderLatencyAverages()
}

func (metrics *OperationalMetrics) recordOrderReject(reason string) {
	reasonKey := normalizedReason(reason)
	metrics.orders.snapshot.Rejected++
	metrics.orders.snapshot.LastReason = reason
	metrics.orders.snapshot.RejectsByReason[reasonKey]++
}

func (metrics *OperationalMetrics) recordOrderCancel(reason string) {
	reasonKey := normalizedReason(reason)
	metrics.orders.snapshot.Canceled++
	metrics.orders.snapshot.LastReason = reason
	metrics.orders.snapshot.CancelsByReason[reasonKey]++
}

func (metrics *OperationalMetrics) clearOrder(clOrdID string) {
	if pendingNotional := metrics.orders.notional[clOrdID]; pendingNotional > 0 {
		metrics.orders.snapshot.PendingExposure -= pendingNotional
	}

	if metrics.orders.snapshot.PendingExposure < 0 {
		metrics.orders.snapshot.PendingExposure = 0
	}

	delete(metrics.orders.submittedAt, clOrdID)
	delete(metrics.orders.notional, clOrdID)
}

func (metrics *OperationalMetrics) refreshOrderLatencyAverages() {
	snapshot := &metrics.orders.snapshot

	if snapshot.Submitted > 0 {
		snapshot.AverageSubmitLatency = metrics.orders.submitLatency /
			time.Duration(snapshot.Submitted)
	}

	if snapshot.Acknowledged > 0 {
		snapshot.AverageAckLatency = metrics.orders.ackLatency /
			time.Duration(snapshot.Acknowledged)
	}

	if snapshot.Filled > 0 {
		snapshot.AverageFillLatency = metrics.orders.fillLatency /
			time.Duration(snapshot.Filled)
	}
}

func (metrics *OperationalMetrics) refreshStopLatencyAverages() {
	snapshot := &metrics.stops.snapshot

	if snapshot.ExitSubmitted > 0 {
		snapshot.AverageTriggerToSubmitLatency = metrics.stops.submitLatency /
			time.Duration(snapshot.ExitSubmitted)
	}

	if snapshot.ExitFilled > 0 {
		snapshot.AverageTriggerToFillLatency = metrics.stops.fillLatency /
			time.Duration(snapshot.ExitFilled)
	}
}

func (metrics *OperationalMetrics) busSnapshots() []BusChannelSnapshot {
	snapshots := make([]BusChannelSnapshot, 0, len(metrics.bus))

	for _, stats := range metrics.bus {
		snapshots = append(snapshots, stats.snapshot)
	}

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		left := snapshots[leftIndex].Channel + snapshots[leftIndex].MessageType
		right := snapshots[rightIndex].Channel + snapshots[rightIndex].MessageType

		return left < right
	})

	return snapshots
}

func (metrics *OperationalMetrics) webSocketSnapshots() []WebSocketSnapshot {
	snapshots := make([]WebSocketSnapshot, 0, len(metrics.websockets))

	for _, stats := range metrics.websockets {
		snapshots = append(snapshots, stats.snapshot)
	}

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		return snapshots[leftIndex].Name < snapshots[rightIndex].Name
	})

	return snapshots
}

func (metrics *OperationalMetrics) marketDataSnapshots() []MarketDataSnapshot {
	snapshots := make([]MarketDataSnapshot, 0, len(metrics.marketData))

	for _, stats := range metrics.marketData {
		snapshots = append(snapshots, stats.snapshot)
	}

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		return snapshots[leftIndex].Kind+snapshots[leftIndex].Symbol <
			snapshots[rightIndex].Kind+snapshots[rightIndex].Symbol
	})

	return snapshots
}

func (metrics *OperationalMetrics) exchangeErrorSnapshots() []ExchangeErrorSnapshot {
	snapshots := make([]ExchangeErrorSnapshot, 0, len(metrics.exchangeErrors))

	for _, stats := range metrics.exchangeErrors {
		snapshots = append(snapshots, stats.snapshot)
	}

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		left := snapshots[leftIndex]
		right := snapshots[rightIndex]

		return left.Component+left.Category+left.Code+left.Action <
			right.Component+right.Category+right.Code+right.Action
	})

	return snapshots
}

func (metrics *OperationalMetrics) orderSnapshot() OrderSnapshot {
	snapshot := metrics.orders.snapshot
	snapshot.RejectsByReason = cloneCounts(snapshot.RejectsByReason)
	snapshot.CancelsByReason = cloneCounts(snapshot.CancelsByReason)
	snapshot.Correlations = metrics.orderCorrelations()

	return snapshot
}

func (metrics *OperationalMetrics) stopSnapshot() StopSnapshot {
	snapshot := metrics.stops.snapshot
	snapshot.RepairReasons = cloneCounts(snapshot.RepairReasons)

	return snapshot
}

func (metrics *OperationalMetrics) orderCorrelations() []OrderCorrelation {
	correlations := make([]OrderCorrelation, 0, len(metrics.orders.correlations))

	for _, correlation := range metrics.orders.correlations {
		correlations = append(correlations, correlation)
	}

	sort.Slice(correlations, func(leftIndex int, rightIndex int) bool {
		return correlations[leftIndex].ClOrdID < correlations[rightIndex].ClOrdID
	})

	return correlations
}

func cloneCounts(counts map[string]int64) map[string]int64 {
	if len(counts) == 0 {
		return nil
	}

	clone := make(map[string]int64, len(counts))

	for key, value := range counts {
		clone[key] = value
	}

	return clone
}

func normalizedReason(reason string) string {
	reason = strings.TrimSpace(strings.ToLower(reason))

	if reason == "" {
		return "unknown"
	}

	return reason
}
