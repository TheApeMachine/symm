package types

type Status string

const (
	UNKNOWN           Status = "unknown"
	INITIALIZING      Status = "initializing"
	PENDING           Status = "pending"
	NEW               Status = "new"
	OPEN              Status = "open"
	CLOSED            Status = "closed"
	CANCELLED         Status = "cancelled"
	REJECTED          Status = "rejected"
	EXPIRED           Status = "expired"
	PARTIAL           Status = "partial"
	PARTIAL_FILLED    Status = "partial_filled"
	PARTIAL_CANCELLED Status = "partial_cancelled"
	PARTIAL_REJECTED  Status = "partial_rejected"
	PARTIAL_EXPIRED   Status = "partial_expired"
	FILLED            Status = "filled"
	AMENDED           Status = "amended"
	RESTATED          Status = "restated"
	STATUS            Status = "status"
	READY             Status = "ready"
	BUSY              Status = "busy"
	PRIORITY          Status = "priority"
	CANCELED          Status = "canceled"
	ERROR             Status = "error"
	FATAL             Status = "fatal"
)

type StatusReporter interface {
	Status() Status
	Initialize() error
}

var MarketStatuses = map[string]Status{
	"pending_new":    PENDING,
	"new":            NEW,
	"trade":          OPEN,
	"filled":         FILLED,
	"canceled":       CANCELED,
	"iceberg_filled": FILLED,
	"expired":        EXPIRED,
	"amended":        AMENDED,
	"restated":       RESTATED,
	"status":         STATUS,
}
