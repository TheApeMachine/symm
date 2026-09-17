package core

// ActionType identifies a canonical operation a receiving primitive may support.
type Action uint8

const (
	None Action = iota
	Identify
	Read
	Write
	Execute
)
