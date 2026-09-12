package data

type Identifiable[T any] interface {
	Identify(int) Identifiable[T]
	Identity() int
}
