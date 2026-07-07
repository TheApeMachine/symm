package types

type Status string

const (
	UNKNOWN      Status = "unknown"
	INITIALIZING Status = "initializing"
	PENDING      Status = "pending"
	NEW          Status = "new"
	OPEN         Status = "open"
	CLOSED       Status = "closed"
	PARTIAL      Status = "partial"
	FILLED       Status = "filled"
	READY        Status = "ready"
	CANCELED     Status = "canceled"
	ERROR        Status = "error"
	FATAL        Status = "fatal"
)
