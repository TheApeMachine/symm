package observability

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type BusChannelSnapshot struct {
	Channel     string        `json:"channel"`
	MessageType string        `json:"message_type"`
	Sent        int64         `json:"sent"`
	Received    int64         `json:"received"`
	Dropped     int64         `json:"dropped"`
	Expired     int64         `json:"expired"`
	Outstanding int64         `json:"outstanding"`
	LastLag     time.Duration `json:"last_lag"`
	LastReason  string        `json:"last_reason,omitempty"`
	ObservedAt  time.Time     `json:"observed_at"`
}

type WebSocketSnapshot struct {
	Name         string    `json:"name"`
	Reconnects   int64     `json:"reconnects"`
	LastReason   string    `json:"last_reason,omitempty"`
	ObservedAt   time.Time `json:"observed_at"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	LastFailure  time.Time `json:"last_failure,omitempty"`
	LastEndpoint string    `json:"last_endpoint,omitempty"`
}

type MarketDataSnapshot struct {
	Kind       string        `json:"kind"`
	Symbol     string        `json:"symbol"`
	Age        time.Duration `json:"age"`
	ObservedAt time.Time     `json:"observed_at"`
	RecordedAt time.Time     `json:"recorded_at"`
}

type OrderCorrelation struct {
	DecisionID      string `json:"decision_id,omitempty"`
	ActionID        string `json:"action_id,omitempty"`
	ClOrdID         string `json:"cl_ord_id,omitempty"`
	ExchangeOrderID string `json:"exchange_order_id,omitempty"`
	ExecutionID     string `json:"execution_id,omitempty"`
	Symbol          string `json:"symbol,omitempty"`
}

type OrderSnapshot struct {
	Submitted            int64              `json:"submitted"`
	Acknowledged         int64              `json:"acknowledged"`
	Filled               int64              `json:"filled"`
	Rejected             int64              `json:"rejected"`
	Canceled             int64              `json:"canceled"`
	AverageSubmitLatency time.Duration      `json:"average_submit_latency"`
	AverageAckLatency    time.Duration      `json:"average_ack_latency"`
	AverageFillLatency   time.Duration      `json:"average_fill_latency"`
	RejectsByReason      map[string]int64   `json:"rejects_by_reason,omitempty"`
	CancelsByReason      map[string]int64   `json:"cancels_by_reason,omitempty"`
	Correlations         []OrderCorrelation `json:"correlations,omitempty"`
	PendingExposure      float64            `json:"pending_exposure"`
	LastReason           string             `json:"last_reason,omitempty"`
	ObservedAt           time.Time          `json:"observed_at"`
}

type StopSnapshot struct {
	Triggered                     int64            `json:"triggered"`
	ExitSubmitted                 int64            `json:"exit_submitted"`
	ExitFilled                    int64            `json:"exit_filled"`
	NeedsRepair                   int64            `json:"needs_repair"`
	AverageTriggerToSubmitLatency time.Duration    `json:"average_trigger_to_submit_latency"`
	AverageTriggerToFillLatency   time.Duration    `json:"average_trigger_to_fill_latency"`
	RepairReasons                 map[string]int64 `json:"repair_reasons,omitempty"`
	ObservedAt                    time.Time        `json:"observed_at"`
}

type ExposureSnapshot struct {
	Currency        string    `json:"currency"`
	OpenPositions   int       `json:"open_positions"`
	OpenExposure    float64   `json:"open_exposure"`
	PendingExposure float64   `json:"pending_exposure"`
	UnrealizedPnL   float64   `json:"unrealized_pnl"`
	ObservedAt      time.Time `json:"observed_at"`
}

type AuditSnapshot struct {
	WriteFailures int64     `json:"write_failures"`
	LastError     string    `json:"last_error,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
}

type ExchangeErrorSnapshot struct {
	Component  string    `json:"component"`
	Category   string    `json:"category"`
	Code       string    `json:"code,omitempty"`
	Action     string    `json:"action"`
	Count      int64     `json:"count"`
	Message    string    `json:"message,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

type Snapshot struct {
	Bus            []BusChannelSnapshot    `json:"bus"`
	WebSockets     []WebSocketSnapshot     `json:"websockets"`
	MarketData     []MarketDataSnapshot    `json:"market_data"`
	ExchangeErrors []ExchangeErrorSnapshot `json:"exchange_errors"`
	Orders         OrderSnapshot           `json:"orders"`
	Stops          StopSnapshot            `json:"stops"`
	Exposure       ExposureSnapshot        `json:"exposure"`
	Audit          AuditSnapshot           `json:"audit"`
}

type OperationalMetrics struct {
	bus            sync.Map
	websockets     sync.Map
	marketData     sync.Map
	exchangeErrors sync.Map
	orders         orderMetrics
	stops          stopMetrics
	exposure       atomic.Pointer[ExposureSnapshot]
	audit          atomic.Pointer[AuditSnapshot]
}

type busChannelStats struct {
	channel          string
	messageType      string
	sent             atomic.Int64
	received         atomic.Int64
	dropped          atomic.Int64
	expired          atomic.Int64
	outstanding      atomic.Int64
	lastLag          atomic.Int64
	oldestQueuedAt   atomic.Int64
	observedAt       atomic.Int64
	lastReason       atomic.Pointer[string]
}

type webSocketStats struct {
	name         string
	reconnects   atomic.Int64
	lastReason   atomic.Pointer[string]
	observedAt   atomic.Int64
	lastSuccess  atomic.Int64
	lastFailure  atomic.Int64
	lastEndpoint atomic.Pointer[string]
}

type marketDataStats struct {
	snapshot atomic.Pointer[MarketDataSnapshot]
}

type exchangeErrorStats struct {
	component  string
	category   string
	code       string
	action     string
	count      atomic.Int64
	message    atomic.Pointer[string]
	observedAt atomic.Int64
}

type orderMetrics struct {
	submitted       atomic.Int64
	acknowledged    atomic.Int64
	filled          atomic.Int64
	rejected        atomic.Int64
	canceled        atomic.Int64
	pendingExposure atomic.Uint64
	submitLatency   atomic.Int64
	ackLatency      atomic.Int64
	fillLatency     atomic.Int64
	observedAt      atomic.Int64
	lastReason      atomic.Pointer[string]
	rejectsByReason sync.Map
	cancelsByReason sync.Map
	correlations    sync.Map
	submittedAt     sync.Map
	notional        sync.Map
}

type stopMetrics struct {
	triggered     atomic.Int64
	exitSubmitted atomic.Int64
	exitFilled    atomic.Int64
	needsRepair   atomic.Int64
	submitLatency atomic.Int64
	fillLatency   atomic.Int64
	observedAt    atomic.Int64
	repairReasons sync.Map
}

var sharedMetrics atomic.Pointer[OperationalMetrics]

func init() {
	sharedMetrics.Store(NewOperationalMetrics())
}

func NewOperationalMetrics() *OperationalMetrics {
	return &OperationalMetrics{}
}

func Shared() *OperationalMetrics {
	return sharedMetrics.Load()
}

func ResetSharedForTest() *OperationalMetrics {
	metrics := NewOperationalMetrics()
	sharedMetrics.Store(metrics)

	return metrics
}

func storeString(slot *atomic.Pointer[string], value string) {
	if value == "" {
		slot.Store(nil)
		return
	}

	stored := value
	slot.Store(&stored)
}

func loadString(slot *atomic.Pointer[string]) string {
	value := slot.Load()

	if value == nil {
		return ""
	}

	return *value
}

func storeTime(slot *atomic.Int64, value time.Time) {
	if value.IsZero() {
		slot.Store(0)
		return
	}

	slot.Store(value.UnixNano())
}

func loadTime(nanos atomic.Int64) time.Time {
	value := nanos.Load()

	if value == 0 {
		return time.Time{}
	}

	return time.Unix(0, value)
}

func storeFloat(slot *atomic.Uint64, value float64) {
	slot.Store(math.Float64bits(value))
}

func loadFloat(slot *atomic.Uint64) float64 {
	return math.Float64frombits(slot.Load())
}

func incrementReasonCount(reasons *sync.Map, reasonKey string) {
	raw, _ := reasons.LoadOrStore(reasonKey, &atomic.Int64{})
	counter, ok := raw.(*atomic.Int64)

	if !ok {
		return
	}

	counter.Add(1)
}

func reasonCounts(reasons *sync.Map) map[string]int64 {
	counts := make(map[string]int64)

	reasons.Range(func(key, value any) bool {
		reasonKey, keyOK := key.(string)
		counter, valueOK := value.(*atomic.Int64)

		if keyOK && valueOK {
			counts[reasonKey] = counter.Load()
		}

		return true
	})

	if len(counts) == 0 {
		return nil
	}

	return counts
}
