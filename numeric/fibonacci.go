package numeric

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
