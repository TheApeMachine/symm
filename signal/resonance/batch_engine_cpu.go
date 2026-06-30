package resonance

import (
	"fmt"

	"github.com/theapemachine/nomagique/learning"
	"golang.org/x/sync/errgroup"
)

type cpuBatchEngine struct {
	slots []*learning.ResonanceManifold
	arch  []int
	alpha float64
}

func newCPUBatchEngine(arch []int, alpha float64, batchSize int) (batchEngine, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("resonance: batch size must be positive")
	}

	engine := &cpuBatchEngine{
		slots: make([]*learning.ResonanceManifold, batchSize),
		arch:  arch,
		alpha: alpha,
	}

	for slotIndex := range batchSize {
		manifold, err := learning.NewResonanceManifold(arch, 0, alpha)

		if err != nil {
			return nil, err
		}

		engine.slots[slotIndex] = manifold
	}

	return engine, nil
}

func (engine *cpuBatchEngine) Reset(slots []int) error {
	if engine == nil {
		return fmt.Errorf("resonance: batch engine is not initialized")
	}

	for _, slot := range slots {
		if slot < 0 || slot >= len(engine.slots) {
			return fmt.Errorf("resonance: reset slot %d out of range", slot)
		}

		// A reused slot must shed the prior symbol's learned weights, not just
		// transient latent state, so a fresh manifold replaces it entirely.
		manifold, err := learning.NewResonanceManifold(engine.arch, 0, engine.alpha)

		if err != nil {
			return err
		}

		engine.slots[slot] = manifold
	}

	return nil
}

func (engine *cpuBatchEngine) Close() {}

func (engine *cpuBatchEngine) Capacity() int {
	return len(engine.slots)
}

func (engine *cpuBatchEngine) Settle(entries []batchEntry) ([]settleOutcome, error) {
	if engine == nil || len(engine.slots) == 0 {
		return nil, fmt.Errorf("resonance: batch engine is not initialized")
	}

	outcomes := make([]settleOutcome, len(entries))
	waitGroup := errgroup.Group{}

	for entryIndex, entry := range entries {
		if entry.slot < 0 || entry.slot >= len(engine.slots) {
			return nil, fmt.Errorf("resonance: slot %d out of range", entry.slot)
		}

		waitGroup.Go(func() error {
			manifold := engine.slots[entry.slot]

			if settleErr := manifold.Settle(entry.input, true); settleErr != nil {
				return settleErr
			}

			manifold.Learn(nil)

			outcomes[entryIndex] = settleOutcome{
				symbol:     entry.symbol,
				input:      entry.input,
				latent:     manifold.LatentState(),
				surprise:   manifold.ReconstructionError(),
				energy:     manifold.Energy(),
				wireSource: manifold,
			}

			return nil
		})
	}

	if waitErr := waitGroup.Wait(); waitErr != nil {
		return nil, waitErr
	}

	return outcomes, nil
}

func newPortableBatchEngine(arch []int, alpha float64, batchSize int) (batchEngine, error) {
	return newCPUBatchEngine(arch, alpha, batchSize)
}
