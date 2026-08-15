//go:build darwin && cgo

package manifold

import "testing"

func TestSolverDarwin_build(testingTB *testing.T) {
	_ = NewSolver
}
