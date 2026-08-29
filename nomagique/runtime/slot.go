package runtime

/*
Slot is the pre-allocated event item living in the ring buffer.
*/
type Slot[T any, U any] struct {
	Payload T
	Results [8]U
}
