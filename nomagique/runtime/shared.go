package runtime

/*
SharedReader lets a signal extract the numeric facts it needs from a shared
object without importing the object's concrete type. A producer that shares an
object also shares the reader that turns it back into the generic values the
signal's pipeline consumes.
*/
type SharedReader interface {
	Fields() []string
	Values(id string) (map[string]float64, bool)
}

/*
SharedResource couples one shared object with the reader that projects it into
generic fields. Producers register this pair; signals fetch the reader and
pull the fields they need, staying ignorant of the concrete object.
*/
type SharedResource struct {
	Object any
	Read   SharedReader
}
