package types

import "time"

/*
EpochObservation identifies one fully completed system epoch.

It is passed by value before Thesis reset, so a supervisor never has to race
the mutable Thesis lifecycle merely to know which completed epoch it observed.
*/
type EpochObservation struct {
	Tick int64
	At   time.Time
}
