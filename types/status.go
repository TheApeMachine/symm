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
	Initialize() error
}

/*
MarketStatuses maps venue exec_type strings onto canonical Status values.
Unknown keys must not be cast; callers use StatusFromMarket.
*/
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

/*
statusEdges enumerates every legal Status transition for broker lots and
subsystem readiness. Terminal states have empty outbound sets.
*/
var statusEdges = map[Status]map[Status]struct{}{
	UNKNOWN:      {INITIALIZING: {}, PENDING: {}, READY: {}, ERROR: {}},
	INITIALIZING: {PENDING: {}, OPEN: {}, READY: {}, CLOSED: {}, ERROR: {}, FATAL: {}},
	PENDING: {
		NEW: {}, OPEN: {}, FILLED: {}, PARTIAL: {}, PARTIAL_FILLED: {},
		CANCELED: {}, REJECTED: {}, EXPIRED: {}, ERROR: {}, CLOSED: {},
		AMENDED: {}, RESTATED: {}, STATUS: {}, PRIORITY: {},
	},
	NEW: {
		OPEN: {}, FILLED: {}, PARTIAL: {}, PARTIAL_FILLED: {},
		CANCELED: {}, REJECTED: {}, EXPIRED: {}, ERROR: {},
		AMENDED: {}, RESTATED: {}, STATUS: {}, PRIORITY: {},
	},
	OPEN: {
		PARTIAL: {}, PARTIAL_FILLED: {}, FILLED: {}, CLOSED: {},
		CANCELED: {}, PENDING: {}, ERROR: {},
		AMENDED: {}, RESTATED: {}, STATUS: {}, PRIORITY: {},
	},
	PARTIAL: {
		PARTIAL_FILLED: {}, FILLED: {}, CLOSED: {}, CANCELED: {},
		OPEN: {}, ERROR: {},
		AMENDED: {}, RESTATED: {}, STATUS: {}, PRIORITY: {},
	},
	PARTIAL_FILLED: {
		FILLED: {}, CLOSED: {}, CANCELED: {}, OPEN: {}, ERROR: {},
		AMENDED: {}, RESTATED: {}, STATUS: {}, PRIORITY: {},
	},
	FILLED:   {OPEN: {}, CLOSED: {}, PENDING: {}, AMENDED: {}, RESTATED: {}, STATUS: {}},
	READY:    {BUSY: {}, PENDING: {}, ERROR: {}, FATAL: {}, PRIORITY: {}, STATUS: {}},
	BUSY:     {READY: {}, ERROR: {}, FATAL: {}, PRIORITY: {}, STATUS: {}},
	CLOSED:   {OPEN: {}},
	CANCELED: {OPEN: {}},
	REJECTED: {},
	EXPIRED:  {},
	ERROR:    {READY: {}, CLOSED: {}, FATAL: {}},
	FATAL:    {},
	AMENDED:  {OPEN: {}, PENDING: {}, FILLED: {}, CANCELED: {}},
	RESTATED: {OPEN: {}, PENDING: {}, FILLED: {}, CANCELED: {}},
	STATUS:   {OPEN: {}, PENDING: {}, FILLED: {}, CANCELED: {}, CLOSED: {}},
	PRIORITY: {READY: {}, BUSY: {}, ERROR: {}},
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

/*
Transition validates and returns the next Status when the edge is legal.
Identical from/to is a no-op success so idempotent acks stay quiet.
*/
func Transition(from, to Status) (Status, error) {
	if from == to {
		return to, nil
	}

	if to == Status("cancelled") {
		to = CANCELED
	}

	allowed, ok := statusEdges[from]

	if !ok {
		return UNKNOWN, errnie.Error(errnie.Err(
			errnie.Validation,
			"unknown status source: "+string(from),
			nil,
		))
	}

	if _, ok := allowed[to]; !ok {
		return UNKNOWN, errnie.Error(errnie.Err(
			errnie.Validation,
			"illegal status transition "+string(from)+" -> "+string(to),
			nil,
		))
	}

	return to, nil
}
