package core

/*
Identifiable provides a way to reference or address external objects.
This can be used to earmark output to a certain external resource,
for example.
*/
type Identifiable[T any] interface {
	Identify(T) Identifiable[T]
	Identity() T
}
