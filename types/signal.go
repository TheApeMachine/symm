package types

/*
Signal measures market rows delivered on Actor topics and appends onto a
shared Thesis. Initialize attaches Live market roots; Run starts from
Actor.Initialize.
*/
type Signal interface {
	Name() string
	Initialize(live *Actor, thesis *Thesis)
}
