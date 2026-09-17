package core

/*
Connectable allows an object to establish a connection to another.
*/
type Connectable[T any] interface {
	Identifiable[T]
	Connect(Primitive)
}
