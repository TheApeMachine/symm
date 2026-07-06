package logic

import (
	"fmt"

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

	return fmt.Sprintf("%s: %s", errBoundaryNoClamps.Error(), err.cause.Error())
}

func (err boundaryNoClampsError) Is(target error) bool {
	return target == errBoundaryNoClamps
}

func (boundaries *boundaryClamps) reject(source types.SourceType, err error) {
	errnie.Error(errnie.Err(
		errnie.Validation,
		fmt.Sprintf("decision boundary: rejected %s clamp: %s", source, err.Error()),
		err,
	))
}
