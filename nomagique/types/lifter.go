package types

/*
Lifter transforms an incoming typed domain event into a generic input Frame.
It is the declarative input endpoint of a signal: the raw market event (a
trade, ticker, or level3 frame) enters, and a populated Frame leaves. The
boolean reports whether the event should be processed at all, so a lifter can
silently drop malformed or unusable rows without leaking domain conditionals
into the pipeline.
*/
type Lifter[T any] interface {
	Lift(event T) (Frame, bool)
}

/*
LifterFunc adapts a plain function to the Lifter interface.
*/
type LifterFunc[T any] func(event T) (Frame, bool)

func (lifter LifterFunc[T]) Lift(event T) (Frame, bool) {
	return lifter(event)
}
