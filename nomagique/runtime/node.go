package runtime

/*
UnitProcessor represents any component that processes 1 item and optionally
emits 1 item. It is completely unaware of channels, concurrency, and mutexes:
the workspace owns the scheduling, the sharding, and the ordering, and calls
Process synchronously in key order.
*/
type UnitProcessor[In, Out any] interface {
	Process(in In) (out Out, emit bool, err error)
}

/*
KeyedUnitProcessor represents a component that processes 1 item for a specific
symbol/key. The workspace guarantees single-threaded sequential execution per
key, so the processor needs no internal synchronization.
*/
type KeyedUnitProcessor[In, Out any] interface {
	ProcessKeyed(key string, in In) (out Out, emit bool, err error)
}

/*
HandlerFunc adapts a plain function into a UnitProcessor so a node can be
declared inline without a dedicated type.
*/
type HandlerFunc[In, Out any] func(in In) (Out, bool, error)

func (handler HandlerFunc[In, Out]) Process(in In) (Out, bool, error) {
	return handler(in)
}
