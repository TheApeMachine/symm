package types

type State uint8

const (
	UNKNOWN State = iota
	INITIALIZING
	READY
)
