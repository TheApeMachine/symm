package relation

import "fmt"

/*
errRelation builds a descriptive relation-layer error. Errors are never
silently swallowed; undefined estimates are carried as FitStatus, while
programming errors (nil store, malformed requests) surface as errors.
*/
func errRelation(format string, arguments ...any) error {
	return fmt.Errorf("relation: "+format, arguments...)
}
