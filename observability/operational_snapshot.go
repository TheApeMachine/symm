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

	exposure := ExposureSnapshot{}
	audit := AuditSnapshot{}

	if stored := metrics.exposure.Load(); stored != nil {
		exposure = *stored
	}

	if stored := metrics.audit.Load(); stored != nil {
		audit = *stored
	}

	return Snapshot{
		Bus:            metrics.busSnapshots(),
		WebSockets:     metrics.webSocketSnapshots(),
		MarketData:     metrics.marketDataSnapshots(),
		ExchangeErrors: metrics.exchangeErrorSnapshots(),
		Orders:         metrics.orderSnapshot(),
		Stops:          metrics.stopSnapshot(),
		Exposure:       exposure,
		Audit:          audit,
	}
}

func (metrics *OperationalMetrics) busSnapshots() []BusChannelSnapshot {
	snapshots := make([]BusChannelSnapshot, 0)

	metrics.bus.Range(func(key, value any) bool {
		stats, ok := value.(*busChannelStats)

		if !ok || stats == nil {
			return true
		}

		snapshots = append(snapshots, BusChannelSnapshot{
			Channel:     stats.channel,
			MessageType: stats.messageType,
			Sent:        stats.sent.Load(),
			Received:    stats.received.Load(),
			Dropped:     stats.dropped.Load(),
			Expired:     stats.expired.Load(),
			Outstanding: stats.outstanding.Load(),
			LastLag:     time.Duration(stats.lastLag.Load()),
			LastReason:  loadString(&stats.lastReason),
			ObservedAt:  loadTime(stats.observedAt),
		})

		return true
	})

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		left := snapshots[leftIndex].Channel + snapshots[leftIndex].MessageType
		right := snapshots[rightIndex].Channel + snapshots[rightIndex].MessageType

		return left < right
	})

	return snapshots
}

func (metrics *OperationalMetrics) webSocketSnapshots() []WebSocketSnapshot {
	snapshots := make([]WebSocketSnapshot, 0)

	metrics.websockets.Range(func(key, value any) bool {
		stats, ok := value.(*webSocketStats)

		if !ok || stats == nil {
			return true
		}

		snapshots = append(snapshots, WebSocketSnapshot{
			Name:         stats.name,
			Reconnects:   stats.reconnects.Load(),
			LastReason:   loadString(&stats.lastReason),
			ObservedAt:   loadTime(stats.observedAt),
			LastSuccess:  loadTime(stats.lastSuccess),
			LastFailure:  loadTime(stats.lastFailure),
			LastEndpoint: loadString(&stats.lastEndpoint),
		})

		return true
	})

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		return snapshots[leftIndex].Name < snapshots[rightIndex].Name
	})

	return snapshots
}

func (metrics *OperationalMetrics) marketDataSnapshots() []MarketDataSnapshot {
	snapshots := make([]MarketDataSnapshot, 0)

	metrics.marketData.Range(func(key, value any) bool {
		stats, ok := value.(*marketDataStats)

		if !ok || stats == nil {
			return true
		}

		if stored := stats.snapshot.Load(); stored != nil {
			snapshots = append(snapshots, *stored)
		}

		return true
	})

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		return snapshots[leftIndex].Kind+snapshots[leftIndex].Symbol <
			snapshots[rightIndex].Kind+snapshots[rightIndex].Symbol
	})

	return snapshots
}

func (metrics *OperationalMetrics) exchangeErrorSnapshots() []ExchangeErrorSnapshot {
	snapshots := make([]ExchangeErrorSnapshot, 0)

	metrics.exchangeErrors.Range(func(key, value any) bool {
		stats, ok := value.(*exchangeErrorStats)

		if !ok || stats == nil {
			return true
		}

		snapshots = append(snapshots, ExchangeErrorSnapshot{
			Component:  stats.component,
			Category:   stats.category,
			Code:       stats.code,
			Action:     stats.action,
			Count:      stats.count.Load(),
			Message:    loadString(&stats.message),
			ObservedAt: loadTime(stats.observedAt),
		})

		return true
	})

	sort.Slice(snapshots, func(leftIndex int, rightIndex int) bool {
		left := snapshots[leftIndex]
		right := snapshots[rightIndex]

		return left.Component+left.Category+left.Code+left.Action <
			right.Component+right.Category+right.Code+right.Action
	})

	return snapshots
}

func (metrics *OperationalMetrics) orderSnapshot() OrderSnapshot {
	orders := metrics.orders
	submitted := orders.submitted.Load()
	acknowledged := orders.acknowledged.Load()
	filled := orders.filled.Load()

	snapshot := OrderSnapshot{
		Submitted:            submitted,
		Acknowledged:         acknowledged,
		Filled:               filled,
		Rejected:             orders.rejected.Load(),
		Canceled:             orders.canceled.Load(),
		PendingExposure:      loadFloat(&orders.pendingExposure),
		LastReason:           loadString(&orders.lastReason),
		ObservedAt:           loadTime(orders.observedAt),
		RejectsByReason:      reasonCounts(&orders.rejectsByReason),
		CancelsByReason:      reasonCounts(&orders.cancelsByReason),
		Correlations:         metrics.orderCorrelations(),
	}

	if submitted > 0 {
		snapshot.AverageSubmitLatency = time.Duration(orders.submitLatency.Load()) / time.Duration(submitted)
	}

	if acknowledged > 0 {
		snapshot.AverageAckLatency = time.Duration(orders.ackLatency.Load()) / time.Duration(acknowledged)
	}

	if filled > 0 {
		snapshot.AverageFillLatency = time.Duration(orders.fillLatency.Load()) / time.Duration(filled)
	}

	return snapshot
}

func (metrics *OperationalMetrics) stopSnapshot() StopSnapshot {
	stops := metrics.stops
	exitSubmitted := stops.exitSubmitted.Load()
	exitFilled := stops.exitFilled.Load()

	snapshot := StopSnapshot{
		Triggered:     stops.triggered.Load(),
		ExitSubmitted: exitSubmitted,
		ExitFilled:    exitFilled,
		NeedsRepair:   stops.needsRepair.Load(),
		ObservedAt:    loadTime(stops.observedAt),
		RepairReasons: reasonCounts(&stops.repairReasons),
	}

	if exitSubmitted > 0 {
		snapshot.AverageTriggerToSubmitLatency =
			time.Duration(stops.submitLatency.Load()) / time.Duration(exitSubmitted)
	}

	if exitFilled > 0 {
		snapshot.AverageTriggerToFillLatency =
			time.Duration(stops.fillLatency.Load()) / time.Duration(exitFilled)
	}

	return snapshot
}

func (metrics *OperationalMetrics) orderCorrelations() []OrderCorrelation {
	correlations := make([]OrderCorrelation, 0)

	metrics.orders.correlations.Range(func(key, value any) bool {
		correlation, ok := value.(OrderCorrelation)

		if ok {
			correlations = append(correlations, correlation)
		}

		return true
	})

	sort.Slice(correlations, func(leftIndex int, rightIndex int) bool {
		return correlations[leftIndex].ClOrdID < correlations[rightIndex].ClOrdID
	})

	return correlations
}

func normalizedReason(reason string) string {
	reason = strings.TrimSpace(strings.ToLower(reason))

	if reason == "" {
		return "unknown"
	}

	return reason
}
