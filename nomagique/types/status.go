package types

type Status uint32

const (
	UNKNOWN Status = iota
	OK
	ERROR
	READY
	BUSY
	STOPPED
	PAUSED
	RESUMED
	DONE
)
