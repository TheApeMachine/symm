//go:build !darwin || !cgo

package manifold

import "fmt"

/*
Engine is unavailable without darwin+cgo Metal.
*/
type Engine struct{}

func NewEngine(config Config) (*Engine, error) {
	return nil, fmt.Errorf("physics: Metal manifold engine requires darwin with cgo enabled")
}

func (engine *Engine) Close() {}

func (engine *Engine) NewField() (*Solver, error) {
	return nil, fmt.Errorf("physics: Metal manifold engine unavailable on this platform")
}

func (engine *Engine) FieldBytes() (uint64, error) {
	return 0, fmt.Errorf("physics: Metal manifold engine unavailable on this platform")
}

func (engine *Engine) MaxFields() (int, error) {
	return 0, fmt.Errorf("physics: Metal manifold engine unavailable on this platform")
}

func (solver *Solver) ResidentBytes() uint64 { return 0 }
