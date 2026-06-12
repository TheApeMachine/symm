package observability

import (
	"strings"
	"time"
)

func (metrics *OperationalMetrics) RecordBusSend(
	channel string,
	messageType string,
	observedAt time.Time,
) {
	if metrics == nil || channel == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	stats := metrics.busStats(channel, messageType)
	stats.snapshot.Sent++
	stats.snapshot.Outstanding++
	stats.snapshot.ObservedAt = observedAt

	if stats.snapshot.Outstanding == 1 {
		stats.oldestQueuedAt = observedAt
	}
}

func (metrics *OperationalMetrics) RecordBusReceive(
	channel string,
	messageType string,
	observedAt time.Time,
) {
	if metrics == nil || channel == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	stats := metrics.busStats(channel, messageType)
	stats.snapshot.Received++
	stats.snapshot.ObservedAt = observedAt

	if stats.snapshot.Outstanding <= 0 {
		return
	}

	if !stats.oldestQueuedAt.IsZero() {
		stats.snapshot.LastLag = observedAt.Sub(stats.oldestQueuedAt)
	}

	stats.snapshot.Outstanding--

	if stats.snapshot.Outstanding == 0 {
		stats.oldestQueuedAt = time.Time{}
		return
	}

	stats.oldestQueuedAt = observedAt
}

func (metrics *OperationalMetrics) RecordBusDrop(
	channel string,
	messageType string,
	reason string,
	observedAt time.Time,
) {
	if metrics == nil || channel == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	stats := metrics.busStats(channel, messageType)
	stats.snapshot.LastReason = reason
	stats.snapshot.ObservedAt = observedAt

	if strings.Contains(strings.ToLower(reason), "expired") {
		stats.snapshot.Expired++
		return
	}

	stats.snapshot.Dropped++
}

func (metrics *OperationalMetrics) RecordWebSocketReconnect(
	name string,
	endpoint string,
	reason string,
	observedAt time.Time,
) {
	if metrics == nil || name == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	stats := metrics.webSocketStats(name)
	stats.snapshot.Reconnects++
	stats.snapshot.LastEndpoint = endpoint
	stats.snapshot.LastReason = reason
	stats.snapshot.LastFailure = observedAt
	stats.snapshot.ObservedAt = observedAt
}

func (metrics *OperationalMetrics) RecordWebSocketConnected(
	name string,
	endpoint string,
	observedAt time.Time,
) {
	if metrics == nil || name == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	stats := metrics.webSocketStats(name)
	stats.snapshot.LastEndpoint = endpoint
	stats.snapshot.LastSuccess = observedAt
	stats.snapshot.ObservedAt = observedAt
}

func (metrics *OperationalMetrics) RecordMarketDataAge(
	kind string,
	symbol string,
	sourceObservedAt time.Time,
	recordedAt time.Time,
) {
	if metrics == nil || kind == "" || symbol == "" || sourceObservedAt.IsZero() {
		return
	}

	if recordedAt.IsZero() || recordedAt.Before(sourceObservedAt) {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	key := kind + ":" + symbol
	stats := metrics.marketData[key]

	if stats == nil {
		stats = &marketDataStats{}
		metrics.marketData[key] = stats
	}

	stats.snapshot = MarketDataSnapshot{
		Kind:       kind,
		Symbol:     symbol,
		Age:        recordedAt.Sub(sourceObservedAt),
		ObservedAt: sourceObservedAt,
		RecordedAt: recordedAt,
	}
}

func (metrics *OperationalMetrics) RecordExchangeError(
	component string,
	category string,
	code string,
	action string,
	message string,
	observedAt time.Time,
) {
	if metrics == nil || component == "" || category == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	key := component + ":" + category + ":" + code + ":" + action
	stats := metrics.exchangeErrors[key]

	if stats == nil {
		stats = &exchangeErrorStats{}
		metrics.exchangeErrors[key] = stats
	}

	stats.snapshot.Component = component
	stats.snapshot.Category = category
	stats.snapshot.Code = code
	stats.snapshot.Action = action
	stats.snapshot.Count++
	stats.snapshot.Message = message
	stats.snapshot.ObservedAt = observedAt
}

func (metrics *OperationalMetrics) RecordOrderSubmitted(
	correlation OrderCorrelation,
	submitLatency time.Duration,
	pendingNotional float64,
	observedAt time.Time,
) {
	if metrics == nil || correlation.ClOrdID == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.orders.snapshot.Submitted++
	metrics.orders.snapshot.ObservedAt = observedAt
	metrics.orders.snapshot.PendingExposure += pendingNotional
	metrics.orders.submitLatency += submitLatency
	metrics.orders.submittedAt[correlation.ClOrdID] = observedAt
	metrics.orders.correlations[correlation.ClOrdID] = correlation
	metrics.orders.notional[correlation.ClOrdID] = pendingNotional
	metrics.refreshOrderLatencyAverages()
}

func (metrics *OperationalMetrics) RecordOrderExecution(
	clOrdID string,
	exchangeOrderID string,
	executionID string,
	status string,
	execType string,
	observedAt time.Time,
) {
	if metrics == nil || clOrdID == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.updateOrderCorrelation(clOrdID, exchangeOrderID, executionID)
	metrics.orders.snapshot.ObservedAt = observedAt
	statusKey := strings.ToLower(strings.TrimSpace(status))

	if execType == "trade" || statusKey == "filled" {
		metrics.recordOrderFill(clOrdID, observedAt)
		return
	}

	if statusKey == "rejected" {
		metrics.recordOrderReject(statusKey)
		metrics.clearOrder(clOrdID)
		return
	}

	if statusKey == "canceled" || statusKey == "cancelled" || statusKey == "expired" {
		metrics.recordOrderCancel(statusKey)
		metrics.clearOrder(clOrdID)
		return
	}

	metrics.recordOrderAck(clOrdID, observedAt)
}

func (metrics *OperationalMetrics) RecordRiskReject(
	symbol string,
	reason string,
	observedAt time.Time,
) {
	if metrics == nil || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	reasonKey := normalizedReason(reason)
	metrics.orders.snapshot.Rejected++
	metrics.orders.snapshot.LastReason = reason
	metrics.orders.snapshot.RejectsByReason[reasonKey]++
	metrics.orders.snapshot.ObservedAt = observedAt
}

func (metrics *OperationalMetrics) RecordStopTriggered(
	symbol string,
	observedAt time.Time,
) {
	if metrics == nil || symbol == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.stops.snapshot.Triggered++
	metrics.stops.snapshot.ObservedAt = observedAt
}

func (metrics *OperationalMetrics) RecordStopExitSubmitted(
	symbol string,
	triggeredAt time.Time,
	observedAt time.Time,
) {
	if metrics == nil || symbol == "" || triggeredAt.IsZero() || observedAt.IsZero() {
		return
	}

	if observedAt.Before(triggeredAt) {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.stops.snapshot.ExitSubmitted++
	metrics.stops.submitLatency += observedAt.Sub(triggeredAt)
	metrics.stops.snapshot.ObservedAt = observedAt
	metrics.refreshStopLatencyAverages()
}

func (metrics *OperationalMetrics) RecordStopExitFilled(
	symbol string,
	triggeredAt time.Time,
	observedAt time.Time,
) {
	if metrics == nil || symbol == "" || triggeredAt.IsZero() || observedAt.IsZero() {
		return
	}

	if observedAt.Before(triggeredAt) {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.stops.snapshot.ExitFilled++
	metrics.stops.fillLatency += observedAt.Sub(triggeredAt)
	metrics.stops.snapshot.ObservedAt = observedAt
	metrics.refreshStopLatencyAverages()
}

func (metrics *OperationalMetrics) RecordStopNeedsRepair(
	symbol string,
	reason string,
	observedAt time.Time,
) {
	if metrics == nil || symbol == "" || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	reasonKey := normalizedReason(reason)
	metrics.stops.snapshot.NeedsRepair++
	metrics.stops.snapshot.RepairReasons[reasonKey]++
	metrics.stops.snapshot.ObservedAt = observedAt
}

func (metrics *OperationalMetrics) RecordExposure(
	currency string,
	openPositions int,
	openExposure float64,
	pendingExposure float64,
	unrealizedPnL float64,
	observedAt time.Time,
) {
	if metrics == nil || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.exposure = ExposureSnapshot{
		Currency:        currency,
		OpenPositions:   openPositions,
		OpenExposure:    openExposure,
		PendingExposure: pendingExposure,
		UnrealizedPnL:   unrealizedPnL,
		ObservedAt:      observedAt,
	}
}

func (metrics *OperationalMetrics) RecordAuditWriteFailure(
	err error,
	observedAt time.Time,
) {
	if metrics == nil || err == nil || observedAt.IsZero() {
		return
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.audit.WriteFailures++
	metrics.audit.LastError = err.Error()
	metrics.audit.ObservedAt = observedAt
}
