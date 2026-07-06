package logic

import (
	"errors"
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type boundaryNoClampsError struct {
	cause error
}

func (err boundaryNoClampsError) Error() string {
	if err.cause == nil {
		return errBoundaryNoClamps.Error()
	}

	return fmt.Sprintf("%s: %s", errBoundaryNoClamps.Error(), err.details())
}

func (err boundaryNoClampsError) Is(target error) bool {
	return target == errBoundaryNoClamps
}

func (err boundaryNoClampsError) details() string {
	messages := make([]string, 0)

	for cause := err.cause; cause != nil; cause = errors.Unwrap(cause) {
		message := strings.TrimSpace(cause.Error())
		if message == "" {
			continue
		}

		messages = append(messages, message)
	}

	return strings.Join(messages, ": ")
}

func (boundaries *boundaryClamps) reject(source types.SourceType, err error) {
	errnie.Error(errnie.Err(
		errnie.Validation,
		fmt.Sprintf("decision boundary: rejected %s clamp: %s", source, err.Error()),
		err,
	))
}
