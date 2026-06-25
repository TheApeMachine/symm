package manifold

import (
	"fmt"
	"time"

	mkernel "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/signal/compute"
)

func (field *Field) ensureSolver() error {
	if field == nil {
		return fmt.Errorf("manifold: field is nil")
	}

	if field.solver != nil {
		return nil
	}

	var solver *mkernel.Solver
	var solverErr error

	gateErr := compute.WithMetalInit(func() error {
		solver, solverErr = mkernel.NewSolver(field.config)

		return solverErr
	})

	if gateErr != nil {
		return gateErr
	}

	if solverErr != nil {
		return solverErr
	}

	field.solver = solver

	return nil
}

func (field *Field) closeSolver() {
	if field == nil || field.solver == nil {
		return
	}

	field.solver.Close()
	field.solver = nil
}

func (field *Field) recreateSolver() error {
	field.closeSolver()

	var solver *mkernel.Solver
	var solverErr error

	gateErr := compute.WithMetalInit(func() error {
		solver, solverErr = mkernel.NewSolver(field.config)

		return solverErr
	})

	if gateErr != nil {
		return gateErr
	}

	if solverErr != nil {
		return solverErr
	}

	field.solver = solver
	field.lastReading = mkernel.Reading{}
	field.lastCarriers = nil
	field.lastRecreateAt = time.Now()

	return nil
}

func (field *Field) Close() {
	field.closeSolver()
}
