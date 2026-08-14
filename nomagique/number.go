package nomagique

import "github.com/theapemachine/symm/nomagique/types"

/*
Number writes the datapoint into the primitive, then reads the computed result.
*/
func Number[T comparable](datapoint types.Input[T], primitive types.IO[T]) types.Output[T] {
	primitive.Write(datapoint)

	return primitive.Read()
}
