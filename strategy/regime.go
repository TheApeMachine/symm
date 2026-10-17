package strategy

import "math"

/*
regime is one symbol's metric energy, the sum of squared self-normalized moves.
A sentence is one regime. It ends when a split index exists whose two-part code
is strictly shorter than the code of the whole series:

	one regime:  (n/2) ln(σ²) + (1/2) ln(n)
	two regimes: that cost for each side, plus ln(n) for the split index

σ² is the mean squared deviation from the segment mean. A segment shorter than
two samples has no variance, so it is not a candidate. Equal lengths do not
split. Among equal two-regime lengths, the latest index is the change point.
*/
type regime struct {
	sum   []float64
	sumSq []float64
}

func (regime *regime) advance(energy float64) bool {
	if len(regime.sum) == 0 {
		regime.sum = []float64{0}
		regime.sumSq = []float64{0}
	}

	last := len(regime.sum) - 1
	regime.sum = append(regime.sum, regime.sum[last]+energy)
	regime.sumSq = append(regime.sumSq, regime.sumSq[last]+energy*energy)

	k, ok := regime.change()
	if !ok {
		return false
	}

	regime.keepAfter(k)
	return true
}

/*
change returns the latest split whose two-regime code is strictly shorter
than the single regime. k is the last index (1-based) of the regime that ends.
*/
func (regime *regime) change() (int, bool) {
	n := len(regime.sum) - 1
	if n < 4 {
		return 0, false
	}

	whole := segmentCost(regime.sum, regime.sumSq, 1, n)
	if math.IsInf(whole, -1) {
		return 0, false
	}

	location := math.Log(float64(n))
	bestK := 0
	best := whole

	for k := 2; k <= n-2; k++ {
		two := segmentCost(regime.sum, regime.sumSq, 1, k) +
			segmentCost(regime.sum, regime.sumSq, k+1, n) +
			location
		if two < best || (bestK > 0 && two == best) {
			best = two
			bestK = k
		}
	}

	if bestK == 0 {
		return 0, false
	}

	return bestK, true
}

func (regime *regime) keepAfter(k int) {
	n := len(regime.sum) - 1
	sum := []float64{0}
	sumSq := []float64{0}
	var total, squares float64

	for i := k + 1; i <= n; i++ {
		energy := regime.sum[i] - regime.sum[i-1]
		total += energy
		squares += energy * energy
		sum = append(sum, total)
		sumSq = append(sumSq, squares)
	}

	regime.sum = sum
	regime.sumSq = sumSq
}

/*
segmentCost is the two-part code length of samples left..right (1-based).
A flat segment's residual is bounded by the squared quantization step of its
mean energy, so the cost stays finite and still below a noisy segment.
*/
func segmentCost(sum, sumSq []float64, left, right int) float64 {
	n := right - left + 1
	if n < 2 {
		return math.Inf(1)
	}

	span := sum[right] - sum[left-1]
	squares := sumSq[right] - sumSq[left-1]
	width := float64(n)
	rss := squares - span*span/width
	mean := span / width
	// The variance of a flat segment cancels to a few ulps. The quantization
	// step that covers that cancellation, squared, is the smallest variance
	// a segment of this energy can claim.
	quantum := math.Abs(mean) * math.Sqrt(8*0x1p-52)

	if quantum == 0 || math.IsNaN(quantum) || math.IsInf(quantum, 0) {
		quantum = math.SmallestNonzeroFloat64
	}

	floor := quantum * quantum

	if floor == 0 || math.IsNaN(floor) {
		floor = math.SmallestNonzeroFloat64
	}

	variance := rss / width

	if variance < floor || math.IsNaN(variance) {
		variance = floor
	}

	return 0.5*width*math.Log(variance) + 0.5*math.Log(width)
}

/*
phraseInSentence reports whether phrase is the sentence or a rule inside it.
A rule whose terminals reach past the break is not this sentence.
*/
func phraseInSentence(phrase, sentence []string) bool {
	if len(phrase) == 0 || len(phrase) > len(sentence) {
		return false
	}

	offset := len(sentence) - len(phrase)

	for i, token := range phrase {
		if sentence[offset+i] != token {
			return false
		}
	}

	return true
}
