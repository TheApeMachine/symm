package sensorium

import (
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
		file.Close()
		os.Remove(path)
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: write metallib",
			err,
		))
	}

	if err := file.Close(); err != nil {
		os.Remove(path)
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: close metallib",
			err,
		))
	}

	engine, err := NewEngine(path, gx, gy, gz, spacing)
	os.Remove(path)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"manifold: new engine",
			err,
		))
	}

	return engine, nil
}
