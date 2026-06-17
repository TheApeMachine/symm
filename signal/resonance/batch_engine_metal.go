//go:build darwin && cgo

package resonance

import (
	"fmt"

	"github.com/theapemachine/nomagique/learning/manifold"
	"github.com/theapemachine/symm/signal/compute"
)

type metalBatchEngine struct {
	solver       *manifold.BatchSolver
	inputDim     int
	inputStaging []float64
}

func newBatchEngine(arch []int, alpha float64, batchSize int) (batchEngine, error) {
	var engine *metalBatchEngine
	var initErr error

	gateErr := compute.WithMetalInit(func() error {
		solver, err := manifold.NewBatchSolver(arch, 0, batchSize, alpha)

		if err != nil {
			initErr = err

			return err
		}

		engine = &metalBatchEngine{
			solver:       solver,
			inputDim:     arch[0],
			inputStaging: make([]float64, batchSize*arch[0]),
		}

		return nil
	})

	if gateErr != nil {
		return nil, gateErr
	}

	if initErr != nil {
		return nil, initErr
	}

	return engine, nil
}

func (engine *metalBatchEngine) Close() {
	if engine == nil || engine.solver == nil {
		return
	}

	engine.solver.Close()
	engine.solver = nil
	engine.inputStaging = nil
}

func (engine *metalBatchEngine) Capacity() int {
	if engine == nil || engine.solver == nil {
		return 0
	}

	return engine.solver.Batch()
}

func (engine *metalBatchEngine) stageInputs(entries []batchEntry) error {
	for _, entry := range entries {
		if entry.slot < 0 || entry.slot >= engine.solver.Batch() {
			return fmt.Errorf("resonance: slot %d out of range", entry.slot)
		}

		if len(entry.input) != engine.inputDim {
			return fmt.Errorf("resonance: input dimension mismatch for slot %d", entry.slot)
		}

		base := entry.slot * engine.inputDim
		copy(engine.inputStaging[base:base+engine.inputDim], entry.input)
	}

	return engine.solver.SetInputs(engine.inputStaging, nil)
}

func (engine *metalBatchEngine) Settle(entries []batchEntry) ([]settleOutcome, error) {
	if engine == nil || engine.solver == nil {
		return nil, fmt.Errorf("resonance: batch engine is not initialized")
	}

	if stageErr := engine.stageInputs(entries); stageErr != nil {
		return nil, stageErr
	}

	if settleErr := engine.solver.Settle(true); settleErr != nil {
		return nil, settleErr
	}

	if learnErr := engine.solver.Learn(); learnErr != nil {
		return nil, learnErr
	}

	if readErr := engine.solver.ReadOutcomes(); readErr != nil {
		return nil, readErr
	}

	outcomes := make([]settleOutcome, len(entries))

	for entryIndex, entry := range entries {
		latent, energy, surprise, outcomeErr := engine.solver.OutcomeSlot(entry.slot)

		if outcomeErr != nil {
			return nil, outcomeErr
		}

		outcomes[entryIndex] = settleOutcome{
			symbol:   entry.symbol,
			input:    entry.input,
			latent:   latent,
			surprise: surprise,
			energy:   energy,
		}
	}

	return outcomes, nil
}
