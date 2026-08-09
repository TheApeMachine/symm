package types

import "github.com/theapemachine/errnie"

/*
Status is the canonical broker and subsystem lifecycle vocabulary.
Canceled is the sole cancel spelling; cancelled is rejected at Transition.
*/
type Status string

const (
	UNKNOWN           Status = "unknown"
	INITIALIZING      Status = "initializing"
	PENDING           Status = "pending"
	NEW               Status = "new"
	OPEN              Status = "open"
	CLOSED            Status = "closed"
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
	ARMED             Status = "armed"
	TRIGGERED         Status = "triggered"
	BUSY              Status = "busy"
	PRIORITY          Status = "priority"
	CANCELED          Status = "canceled"
	ERROR             Status = "error"
	FATAL             Status = "fatal"
)

/*
StatusReporter is the Initialize/Status contract for boot-ordered components.
*/
type StatusReporter interface {
	Status() Status
}

/*
MarketStatuses maps venue exec_type strings onto canonical Status values.
Unknown keys must not be cast; callers use StatusFromMarket.
*/
var MarketStatuses = map[string]Status{
	"pending_new":      PENDING,
	"new":              NEW,
	"trade":            OPEN,
	"partially_filled": PARTIAL_FILLED,
	"filled":           FILLED,
	"canceled":         CANCELED,
	"iceberg_filled":   FILLED,
	"expired":          EXPIRED,
	"amended":          AMENDED,
	"restated":         RESTATED,
	"status":           STATUS,
}

/*
StatusFromMarket resolves a venue exec_type into a canonical Status.
Unknown types reject rather than inventing a Status string.
*/
func StatusFromMarket(execType string) (Status, error) {
	status, ok := MarketStatuses[execType]

	if !ok {
		return UNKNOWN, errnie.Error(errnie.Err(
			errnie.Validation,
			"unknown market status: "+execType,
			nil,
		))
	}

	return status, nil
}
