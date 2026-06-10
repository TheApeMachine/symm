package internal

import (
	"context"
	"errors"

	"github.com/theapemachine/errnie"
)

/*
IsShutdown reports expected termination from context cancellation.
*/
func IsShutdown(err error) bool {
	return errors.Is(err, context.Canceled)
}

/*
ReportError logs unexpected errors and passes shutdown errors through silently.
*/
func ReportError(err error) error {
	if err == nil || IsShutdown(err) {
		return err
	}

	return errnie.Error(err)
}
