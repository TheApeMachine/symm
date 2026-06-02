package optimizer

import "runtime"

/*
DefaultTuneOptions returns a tune profile with limits left unset so they are
derived from the measurement tape at run time.
*/
func DefaultTuneOptions(workers int) TuneOptions {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	return TuneOptions{
		Workers: workers,
		Hybrid:  true,
		Guard:   GuardOptions{},
	}
}
