package numeric

import "math"

/*
FibWindows is the Fibonacci sequence of window sizes used for multi-scale
co-occurrence and eigen initialization. Small windows (3–8) capture
fine-grained local correlation; larger windows (13–21) capture longer-range
coupling. Works for any token stream — text, images, audio — no modality-specific
assumptions.

Bounds: 3 is the smallest window with non-trivial co-occurrence structure;
21 is an upper limit before the matrix becomes too sparse for reliable
eigenvectors.
*/
var FibWindows = []int{3, 5, 8, 13, 21}

/*
FibWeights are the mixing weights for each Fibonacci window, summing to 1.0.
Derived from FibWindows as 1/window (inverse scale): local correlation is
denser per byte than long-range; smaller windows get higher weight.
*/
var FibWeights []float64

func init() {
	var sum float64

	for _, window := range FibWindows {
		sum += 1.0 / float64(window)
	}

	FibWeights = make([]float64, len(FibWindows))

	for idx, window := range FibWindows {
		FibWeights[idx] = (1.0 / float64(window)) / sum
	}
}

type Numerics struct {
	BasisPrimes []int32
}

func NewNumerics() *Numerics {
	numerics := &Numerics{
		BasisPrimes: make([]int32, 512),
	}

	numerics.SieveOfEratosthenes(4000) // Upper bound for len(BasisPrimes) primes is 3671
	return numerics
}

func (numerics *Numerics) SumSinCos(phases []float64) (float64, float64) {
	sinSum := 0.0
	cosSum := 0.0

	for _, phase := range phases {
		sinSum += math.Sin(phase)
		cosSum += math.Cos(phase)
	}

	return sinSum, cosSum
}

func (numerics *Numerics) CircularDistance(angleA, angleB float64) float64 {
	delta := math.Mod(angleA-angleB+math.Pi, 2*math.Pi)

	if delta < 0 {
		delta += 2 * math.Pi
	}

	return delta - math.Pi
}

func (numerics *Numerics) SieveOfEratosthenes(limit int) {
	checked := make([]bool, limit)
	sqrtLimit := int(math.Sqrt(float64(limit)))

	for p := 2; p <= sqrtLimit; p++ {
		if !checked[p] {
			for multiple := p * p; multiple < limit; multiple += p {
				checked[multiple] = true
			}
		}
	}

	basisCap := len(numerics.BasisPrimes)
	idx := 0

	for candidate := 2; candidate < limit && idx < basisCap; candidate++ {
		if !checked[candidate] {
			numerics.BasisPrimes[idx] = int32(candidate)
			idx++
		}
	}
}
