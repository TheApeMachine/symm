package associative

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewGrid applies sympathetic clustering to the lock-free store Grid.
It tracks metric histories to calculate SNR, applies a "water table"
threshold (Mean + StdDev), and identifies the dominant region.
Returns the byte string identifier of the winning region context.
*/
type Grid types.Value[[]float64, []byte]
func NewGrid() Grid {
	var prev []float64
	var mean []float64
	var m2 []float64
	var count float64

	return func(impulse []float64) []byte {
		if len(impulse) == 0 {
			return nil
		}

		n := len(impulse)
		if prev == nil {
			prev = make([]float64, n)
			mean = make([]float64, n)
			m2 = make([]float64, n)
			copy(prev, impulse)
			copy(mean, impulse)
			count = 1
			return make([]byte, 32)
		}

		count++
		var globalMean, globalM2 float64

		for i, val := range impulse {
			// Update cell statistics for SNR
			delta := val - mean[i]
			mean[i] += delta / count
			delta2 := val - mean[i]
			m2[i] += delta * delta2

			// Global statistics for topological threshold
			gDelta := val - globalMean
			globalMean += gDelta / float64(i+1)
			gDelta2 := val - globalMean
			globalM2 += gDelta * gDelta2
		}

		globalStdDev := 0.0
		if n > 1 {
			globalStdDev = math.Sqrt(globalM2 / float64(n-1))
		}

		// Topological "water table" split
		threshold := globalMean
		if n > 2 {
			threshold += globalStdDev
		}
		
		bestCell := -1
		maxPull := -1.0 // support zero or negative pull if needed, though impulse should be pos

		for i, val := range impulse {
			if val > threshold {
				variance := 0.0
				if count > 1 {
					variance = m2[i] / (count - 1)
				}

				snr := 0.0
				if variance > 0 {
					snr = math.Abs(mean[i]) / math.Sqrt(variance)
				}

				// Sympathetic priority: SNR powers attraction
				pull := val * snr
				if pull > maxPull {
					maxPull = pull
					bestCell = i
				}
			}
		}

		copy(prev, impulse)

		if bestCell >= 0 {
			return fmt.Appendf(nil, "r%d", bestCell)
		}

		return nil
	}
}
