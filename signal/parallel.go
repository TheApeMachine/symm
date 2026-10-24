package signal

import "runtime"

// WorkerLimit bounds fan-out for per-trade/per-symbol signal work.
func WorkerLimit() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}

	return n
}
