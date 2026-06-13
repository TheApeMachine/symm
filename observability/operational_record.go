package observability

import (
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
)

func (metrics *OperationalMetrics) loadBusStats(
	channel string,
	messageType string,
) *busChannelStats {
	key := channel + ":" + messageType

	if raw, ok := metrics.bus.Load(key); ok {
		stats, statsOK := raw.(*busChannelStats)

		if statsOK {
			return stats
		}
	}

	stats := &busChannelStats{
		channel:     channel,
		messageType: messageType,
	}
	actual, _ := metrics.bus.LoadOrStore(key, stats)

	loaded, ok := actual.(*busChannelStats)

	if ok {
		return loaded
	}

	return stats
}

func (metrics *OperationalMetrics) loadWebSocketStats(name string) *webSocketStats {
	if raw, ok := metrics.websockets.Load(name); ok {
		stats, statsOK := raw.(*webSocketStats)

		if statsOK {
			return stats
		}
	}

	stats := &webSocketStats{name: name}
	actual, _ := metrics.websockets.LoadOrStore(name, stats)

	loaded, ok := actual.(*webSocketStats)

	if ok {
		return loaded
	}

	return stats
}

func (metrics *OperationalMetrics) loadMarketDataStats(key string) *marketDataStats {
	if raw, ok := metrics.marketData.Load(key); ok {
		stats, statsOK := raw.(*marketDataStats)

		if statsOK {
			return stats
		}
	}

	stats := &marketDataStats{}
	actual, _ := metrics.marketData.LoadOrStore(key, stats)

	loaded, ok := actual.(*marketDataStats)

	if ok {
		return loaded
	}

	return stats
}

func (metrics *OperationalMetrics) loadExchangeErrorStats(key string) *exchangeErrorStats {
	if raw, ok := metrics.exchangeErrors.Load(key); ok {
		stats, statsOK := raw.(*exchangeErrorStats)

		if statsOK {
			return stats
		}
	}

	stats := &exchangeErrorStats{}
	actual, _ := metrics.exchangeErrors.LoadOrStore(key, stats)

	loaded, ok := actual.(*exchangeErrorStats)

	if ok {
		return loaded
	}

	return stats
}

func (metrics *OperationalMetrics) RecordBusSend(
	channel string,
	messageType string,
	observedAt time.Time,
) {
	if metrics == nil || channel == "" || observedAt.IsZero() {
		return
	}

	stats := metrics.loadBusStats(channel, messageType)
	stats.sent.Add(1)
	outstanding := stats.outstanding.Add(1)
	storeTime(&stats.observedAt, observedAt)

	if outstanding == 1 {
		storeTime(&stats.oldestQueuedAt, observedAt)
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

	stats := metrics.loadBusStats(channel, messageType)
	stats.received.Add(1)
	storeTime(&stats.observedAt, observedAt)

	if stats.outstanding.Load() <= 0 {
		return
	}

	oldestQueuedAt := loadTime(stats.oldestQueuedAt)

	if !oldestQueuedAt.IsZero() {
		stats.lastLag.Store(observedAt.Sub(oldestQueuedAt).Nanoseconds())
	}

	if stats.outstanding.Add(-1) == 0 {
		stats.oldestQueuedAt.Store(0)
	}
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

	stats := metrics.loadBusStats(channel, messageType)
	storeString(&stats.lastReason, reason)
	storeTime(&stats.observedAt, observedAt)

	if strings.Contains(strings.ToLower(reason), "expired") {
		stats.expired.Add(1)
		return
	}

	stats.dropped.Add(1)
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

	stats := metrics.loadWebSocketStats(name)
	stats.reconnects.Add(1)
	storeString(&stats.lastEndpoint, endpoint)
	storeString(&stats.lastReason, reason)
	storeTime(&stats.lastFailure, observedAt)
	storeTime(&stats.observedAt, observedAt)
}

func (metrics *OperationalMetrics) RecordWebSocketConnected(
	name string,
	endpoint string,
	observedAt time.Time,
) {
	if metrics == nil || name == "" || observedAt.IsZero() {
		return
	}

	stats := metrics.loadWebSocketStats(name)
	storeString(&stats.lastEndpoint, endpoint)
	storeTime(&stats.lastSuccess, observedAt)
	storeTime(&stats.observedAt, observedAt)
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

	key := kind + ":" + symbol
	stats := metrics.loadMarketDataStats(key)
	snapshot := MarketDataSnapshot{
		Kind:       kind,
		Symbol:     symbol,
		Age:        recordedAt.Sub(sourceObservedAt),
		ObservedAt: sourceObservedAt,
		RecordedAt: recordedAt,
	}
	stats.snapshot.Store(&snapshot)
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

	key := component + ":" + category + ":" + code + ":" + action
	stats := metrics.loadExchangeErrorStats(key)
	stats.component = component
	stats.category = category
	stats.code = code
	stats.action = action
	stats.count.Add(1)
	storeString(&stats.message, message)
	storeTime(&stats.observedAt, observedAt)
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

	orders := &metrics.orders
	orders.submitted.Add(1)
	storeTime(&orders.observedAt, observedAt)
	addFloat(&orders.pendingExposure, pendingNotional)
	orders.submitLatency.Add(submitLatency.Nanoseconds())
	orders.submittedAt.Store(correlation.ClOrdID, observedAt.UnixNano())
	orders.correlations.Store(correlation.ClOrdID, correlation)
	orders.notional.Store(correlation.ClOrdID, math.Float64bits(pendingNotional))
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

	orders := &metrics.orders
	updateOrderCorrelation(orders, clOrdID, exchangeOrderID, executionID)
	storeTime(&orders.observedAt, observedAt)
	statusKey := strings.ToLower(strings.TrimSpace(status))

	if execType == "trade" || statusKey == "filled" {
		recordOrderFill(orders, clOrdID, observedAt)
		return
	}

	if statusKey == "rejected" {
		recordOrderReject(orders, statusKey)
		clearOrder(orders, clOrdID)
		return
	}

	if statusKey == "canceled" || statusKey == "cancelled" || statusKey == "expired" {
		recordOrderCancel(orders, statusKey)
		clearOrder(orders, clOrdID)
		return
	}

	recordOrderAck(orders, clOrdID, observedAt)
}

func (metrics *OperationalMetrics) RecordRiskReject(
	symbol string,
	reason string,
	observedAt time.Time,
) {
	if metrics == nil || observedAt.IsZero() {
		return
	}

	orders := &metrics.orders
	reasonKey := normalizedReason(reason)
	orders.rejected.Add(1)
	storeString(&orders.lastReason, reason)
	incrementReasonCount(&orders.rejectsByReason, reasonKey)
	storeTime(&orders.observedAt, observedAt)
}

func (metrics *OperationalMetrics) RecordStopTriggered(
	symbol string,
	observedAt time.Time,
) {
	if metrics == nil || symbol == "" || observedAt.IsZero() {
		return
	}

	metrics.stops.triggered.Add(1)
	storeTime(&metrics.stops.observedAt, observedAt)
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

	stops := &metrics.stops
	stops.exitSubmitted.Add(1)
	stops.submitLatency.Add(observedAt.Sub(triggeredAt).Nanoseconds())
	storeTime(&stops.observedAt, observedAt)
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

	stops := &metrics.stops
	stops.exitFilled.Add(1)
	stops.fillLatency.Add(observedAt.Sub(triggeredAt).Nanoseconds())
	storeTime(&stops.observedAt, observedAt)
}

func (metrics *OperationalMetrics) RecordStopNeedsRepair(
	symbol string,
	reason string,
	observedAt time.Time,
) {
	if metrics == nil || symbol == "" || observedAt.IsZero() {
		return
	}

	stops := &metrics.stops
	reasonKey := normalizedReason(reason)
	stops.needsRepair.Add(1)
	incrementReasonCount(&stops.repairReasons, reasonKey)
	storeTime(&stops.observedAt, observedAt)
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

	snapshot := ExposureSnapshot{
		Currency:        currency,
		OpenPositions:   openPositions,
		OpenExposure:    openExposure,
		PendingExposure: pendingExposure,
		UnrealizedPnL:   unrealizedPnL,
		ObservedAt:      observedAt,
	}
	metrics.exposure.Store(&snapshot)
}

func (metrics *OperationalMetrics) RecordAuditWriteFailure(
	err error,
	observedAt time.Time,
) {
	if metrics == nil || err == nil || observedAt.IsZero() {
		return
	}

	for {
		current := metrics.audit.Load()
		next := AuditSnapshot{
			WriteFailures: 1,
			LastError:     err.Error(),
			ObservedAt:    observedAt,
		}

		if current != nil {
			next.WriteFailures = current.WriteFailures + 1
		}

		if metrics.audit.CompareAndSwap(current, &next) {
			return
		}
	}
}

func updateOrderCorrelation(
	orders *orderMetrics,
	clOrdID string,
	exchangeOrderID string,
	executionID string,
) {
	raw, ok := orders.correlations.Load(clOrdID)

	correlation := OrderCorrelation{ClOrdID: clOrdID}

	if ok {
		stored, storedOK := raw.(OrderCorrelation)

		if storedOK {
			correlation = stored
		}
	}

	if exchangeOrderID != "" {
		correlation.ExchangeOrderID = exchangeOrderID
	}

	if executionID != "" {
		correlation.ExecutionID = executionID
	}

	orders.correlations.Store(clOrdID, correlation)
}

func recordOrderAck(
	orders *orderMetrics,
	clOrdID string,
	observedAt time.Time,
) {
	orders.acknowledged.Add(1)

	if submittedAt, ok := orders.submittedAt.Load(clOrdID); ok {
		if submittedNanos, submittedOK := submittedAt.(int64); submittedOK && submittedNanos > 0 {
			orders.ackLatency.Add(observedAt.Sub(time.Unix(0, submittedNanos)).Nanoseconds())
		}
	}
}

func recordOrderFill(
	orders *orderMetrics,
	clOrdID string,
	observedAt time.Time,
) {
	orders.filled.Add(1)

	if submittedAt, ok := orders.submittedAt.Load(clOrdID); ok {
		if submittedNanos, submittedOK := submittedAt.(int64); submittedOK && submittedNanos > 0 {
			orders.fillLatency.Add(observedAt.Sub(time.Unix(0, submittedNanos)).Nanoseconds())
		}
	}

	clearOrder(orders, clOrdID)
}

func recordOrderReject(orders *orderMetrics, reason string) {
	reasonKey := normalizedReason(reason)
	orders.rejected.Add(1)
	storeString(&orders.lastReason, reason)
	incrementReasonCount(&orders.rejectsByReason, reasonKey)
}

func recordOrderCancel(orders *orderMetrics, reason string) {
	reasonKey := normalizedReason(reason)
	orders.canceled.Add(1)
	storeString(&orders.lastReason, reason)
	incrementReasonCount(&orders.cancelsByReason, reasonKey)
}

func clearOrder(orders *orderMetrics, clOrdID string) {
	if raw, ok := orders.notional.Load(clOrdID); ok {
		if pendingBits, pendingOK := raw.(uint64); pendingOK {
			pendingNotional := math.Float64frombits(pendingBits)

			if pendingNotional > 0 {
				nextExposure := loadFloat(&orders.pendingExposure) - pendingNotional

				if nextExposure < 0 {
					errnie.Info(
						"observability: negative pending exposure reset",
						"cl_ord_id",
						clOrdID,
						"pending_exposure",
						nextExposure,
					)
					nextExposure = 0
				}

				storeFloat(&orders.pendingExposure, nextExposure)
			}
		}
	}

	orders.submittedAt.Delete(clOrdID)
	orders.notional.Delete(clOrdID)
}

func addFloat(slot *atomic.Uint64, delta float64) {
	for {
		current := loadFloat(slot)
		next := current + delta

		if slot.CompareAndSwap(math.Float64bits(current), math.Float64bits(next)) {
			return
		}
	}
}
