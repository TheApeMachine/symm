package types

type IO[T comparable] interface {
	Read() Output[T]
	Write(Input[T])
	Project() Value[T]
	Error() string
	Close() error
}
