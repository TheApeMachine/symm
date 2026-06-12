package observability

import (
	"sync"
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
	mu             sync.RWMutex
	bus            map[string]*busChannelStats
	websockets     map[string]*webSocketStats
	marketData     map[string]*marketDataStats
	exchangeErrors map[string]*exchangeErrorStats
	orders         orderStats
	stops          stopStats
	exposure       ExposureSnapshot
	audit          AuditSnapshot
}

type busChannelStats struct {
	snapshot       BusChannelSnapshot
	oldestQueuedAt time.Time
}

type webSocketStats struct {
	snapshot WebSocketSnapshot
}

type marketDataStats struct {
	snapshot MarketDataSnapshot
}

type exchangeErrorStats struct {
	snapshot ExchangeErrorSnapshot
}

type orderStats struct {
	snapshot      OrderSnapshot
	submittedAt   map[string]time.Time
	correlations  map[string]OrderCorrelation
	notional      map[string]float64
	submitLatency time.Duration
	ackLatency    time.Duration
	fillLatency   time.Duration
}

type stopStats struct {
	snapshot      StopSnapshot
	submitLatency time.Duration
	fillLatency   time.Duration
}

var sharedMetrics = NewOperationalMetrics()

func NewOperationalMetrics() *OperationalMetrics {
	return &OperationalMetrics{
		bus:            make(map[string]*busChannelStats),
		websockets:     make(map[string]*webSocketStats),
		marketData:     make(map[string]*marketDataStats),
		exchangeErrors: make(map[string]*exchangeErrorStats),
		orders: orderStats{
			submittedAt:  make(map[string]time.Time),
			correlations: make(map[string]OrderCorrelation),
			notional:     make(map[string]float64),
			snapshot: OrderSnapshot{
				RejectsByReason: make(map[string]int64),
				CancelsByReason: make(map[string]int64),
			},
		},
		stops: stopStats{
			snapshot: StopSnapshot{
				RepairReasons: make(map[string]int64),
			},
		},
	}
}

func Shared() *OperationalMetrics {
	return sharedMetrics
}

func ResetSharedForTest() *OperationalMetrics {
	sharedMetrics = NewOperationalMetrics()

	return sharedMetrics
}
