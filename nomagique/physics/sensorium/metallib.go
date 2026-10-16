//go:build darwin && cgo

package sensorium

import (
	"errors"
	"os"

	_ "embed"

	"github.com/theapemachine/errnie"
)

//go:embed kernels.metallib
var kernelMetallib []byte

/*
NewEmbeddedEngine loads the package-built kernels.metallib into a Metal context.
*/
func NewEmbeddedEngine(gx, gy, gz int, spacing float32) (*Engine, error) {
	if len(kernelMetallib) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: kernels.metallib is empty",
			nil,
		))
	}

	file, err := os.CreateTemp("", "symm-sensorium-*.metallib")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: temp metallib",
			err,
		))
	}

	path := file.Name()

	if _, err := file.Write(kernelMetallib); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: write metallib",
			errors.Join(err, file.Close(), os.Remove(path)),
		))
	}

	if err := file.Close(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: close metallib",
			errors.Join(err, os.Remove(path)),
		))
	}

	engine, err := NewEngine(path, gx, gy, gz, spacing)

	if removeErr := os.Remove(path); removeErr != nil {
		errnie.Error(errnie.Err(errnie.IO, "manifold: remove temp metallib "+path, removeErr))
	}

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: new engine",
			err,
		))
	}

	return engine, nil
}
