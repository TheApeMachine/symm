//go:build (!darwin && (!linux || !cuda)) || !cgo

package sensorium

func NewEmbeddedEngine(gx, gy, gz int, spacing float32) (*Engine, error) {
	return NewEngine("", gx, gy, gz, spacing)
}
