package budget

import (
	"runtime"

	"github.com/theapemachine/symm/optimizer/types"
)

/*
DefaultTuneOptions returns a tune profile with limits left unset so they are
derived from the measurement tape at run time.
*/
func DefaultTuneOptions(workers int) types.TuneOptions {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	return types.TuneOptions{
		Workers: workers,
		Hybrid:  true,
		Guard:   types.GuardOptions{},
	}
}
