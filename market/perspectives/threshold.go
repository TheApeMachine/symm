package perspectives

import "sync"

var (
	noiseFloorMu         sync.RWMutex
	noiseFloorSNRRuntime = 1.0
)

/*
SetNoiseFloorSNR updates the live SNR gate threshold used by decision trees.
*/
func SetNoiseFloorSNR(value float64) {
	if value <= 0 {
		return
	}

	noiseFloorMu.Lock()
	noiseFloorSNRRuntime = value
	noiseFloorMu.Unlock()
}

/*
NoiseFloorSNR returns the live SNR gate threshold.
*/
func NoiseFloorSNR() float64 {
	noiseFloorMu.RLock()
	defer noiseFloorMu.RUnlock()

	return noiseFloorSNRRuntime
}

func snrThreshold() float64 {
	return NoiseFloorSNR()
}
