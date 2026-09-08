package types

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
