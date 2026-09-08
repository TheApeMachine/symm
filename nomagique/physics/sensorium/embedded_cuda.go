//go:build linux && cgo && cuda

package sensorium

// NewEmbeddedEngine uses the compiled CUDA library; no metallib is opened.
func NewEmbeddedEngine(gx, gy, gz int, spacing float32) (*Engine, error) {
	return NewCUDAEngine(0, gx, gy, gz, spacing)
}
