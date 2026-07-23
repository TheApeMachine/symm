package types

/*
Signal measures market rows delivered on Actor topics and appends onto a
shared Thesis. Initialize attaches Live roots; Run starts the Actor loop.
*/
type Signal interface {
	Name() string
	Initialize(live *Actor, thesis *Thesis)
}
