package types

type IO[T comparable] interface {
	Read() IO[T]
	Write(IO[T])
	Project() Value[T]
	Error() string
	Close() error
}
