package stack

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
TestImmutableCutCheckpointRaceStress proves concurrent checkpoint writes into
isolated directories do not race in-memory cut state.
*/
func TestImmutableCutCheckpointRaceStress(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	waitGroup := sync.WaitGroup{}

	for workerIndex := range 4 {
		waitGroup.Add(1)

		go func(index int) {
			defer waitGroup.Done()
			dir := filepath.Join(root, fmt.Sprintf("worker-%d", index))

			for tick := range 16 {
				cut := &types.ImmutableCut{
					ID:   types.CutID(index*16 + tick),
					Tick: int64(tick),
					At:   at,
					Measurements: []*types.Measurement{{
						Symbol: "BTC/USD",
						Source: types.SourceHawkes,
					}},
				}

				if err := cut.Checkpoint(dir); err != nil {
					t.Errorf("checkpoint: %v", err)
				}
			}
		}(workerIndex)
	}

	waitGroup.Wait()
}
