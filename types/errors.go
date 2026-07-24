package types

import "fmt"

/*
ClosedError reports that a component rejected work because it is shutting down
or already closed. Libraries return this without logging so the process boundary
owns policy.
*/
type ClosedError struct {
	Component string
}

/*
Error formats the closed component name.
*/
func (closed ClosedError) Error() string {
	return closed.Component + ": closed"
}

/*
SaturatedError reports that a bounded queue rejected a non-blocking push.
Callers may count this without treating it as a process fault.
*/
type SaturatedError struct {
	Component string
}

/*
Error formats the saturated component name.
*/
func (saturated SaturatedError) Error() string {
	return saturated.Component + ": saturated"
}

/*
VersionError reports an incompatible wire envelope version.
*/
type VersionError struct {
	Want uint16
	Got  uint16
}

/*
Error formats the version mismatch.
*/
func (version VersionError) Error() string {
	return fmt.Sprintf("wire: version %d incompatible with %d", version.Got, version.Want)
}

/*
ValidationError reports a caller contract violation without logging.
*/
type ValidationError struct {
	Component string
	Detail    string
}

/*
Error formats the validation failure.
*/
func (validation ValidationError) Error() string {
	if validation.Component == "" {
		return validation.Detail
	}

	return validation.Component + ": " + validation.Detail
}

/*
ConflictError reports duplicate or overlapping claims.
*/
type ConflictError struct {
	Component string
	Detail    string
}

/*
Error formats the conflict.
*/
func (conflict ConflictError) Error() string {
	if conflict.Component == "" {
		return conflict.Detail
	}

	return conflict.Component + ": " + conflict.Detail
}

/*
NotFoundError reports a missing reservation or lookup without logging.
*/
type NotFoundError struct {
	Component string
	Detail    string
}

/*
Error formats the missing entity.
*/
func (missing NotFoundError) Error() string {
	if missing.Component == "" {
		return missing.Detail
	}

	return missing.Component + ": " + missing.Detail
}
