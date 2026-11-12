package tokenizer

import (
	"math"
)

/*
MDL finds segment boundaries in the symbol stream Sequitur emits (terminals
and rule refs such as R06.R00 and M1), using minimum description length.
*/
type MDL struct {
	stream []uint32
	offset int
	dist   *symbolHistogram

	vocab  map[string]uint32
	nextID uint32

	lastGain float64

	MinSegmentSymbols int
}

/*
NewMDL starts an empty segmenter — MinSegmentSymbols defaults to 2 so MDL ignores
single-symbol noise before declaring a sentence boundary worth a Sequitur.Sentence.
*/
func NewMDL() *MDL {
	return &MDL{
		dist:              newSymbolHistogram(),
		vocab:             make(map[string]uint32),
		nextID:            1,
		MinSegmentSymbols: 2,
	}
}

/*
Analyze ingests one grammar symbol from the active Sequitur root sequence.
It returns whether an MDL cut was committed and the penalized split gain.
*/
func (mdl *MDL) Analyze(symbol string) (cut bool, gain float64) {
	if symbol == "" {
		return false, 0
	}

	id := mdl.intern(symbol)

	mdl.stream = append(mdl.stream, id)
	mdl.dist.add(id)

	seg := mdl.stream[mdl.offset:]
	if len(seg) < 2*mdl.MinSegmentSymbols {
		return false, 0
	}

	ok, k, g := mdl.detectBoundary(seg, mdl.dist)
	if !ok || g <= 0 || k <= 0 || k >= len(seg) {
		return false, 0
	}

	mdl.offset += k
	mdl.dist = newSymbolHistogram()

	// Drop the committed prefix so Persist does not rewrite unbounded history.
	if mdl.offset > 0 {
		remaining := append([]uint32(nil), mdl.stream[mdl.offset:]...)
		mdl.stream = remaining
		mdl.offset = 0
	}

	for _, symID := range mdl.stream {
		mdl.dist.add(symID)
	}

	mdl.lastGain = g

	return true, g
}

/*
intern compresses grammar tail strings to uint32 ids so boundary search avoids
repeated string comparisons in the hot detectBoundary loop.
*/
func (mdl *MDL) intern(symbol string) uint32 {
	id, ok := mdl.vocab[symbol]
	if ok {
		return id
	}

	id = mdl.nextID
	mdl.nextID++
	mdl.vocab[symbol] = id

	return id
}

/*
detectBoundary scans split points for penalized MDL gain — avoids cuts that only
slice noise without a cheaper two-part code for the symbol stream.
*/
func (mdl *MDL) detectBoundary(stream []uint32, dist *symbolHistogram) (bool, int, float64) {
	n := len(stream)
	minSeg := max(mdl.MinSegmentSymbols, 2)

	if n < 2*minSeg {
		return false, 0, 0
	}

	bestK := 0
	maxGain := 0.0

	costFull := dist.cost()
	left := newSymbolHistogram()
	right := dist.clone()

	for i := 1; i < n; i++ {
		symID := stream[i-1]

		left.add(symID)
		right.remove(symID)

		if i < minSeg || n-i < minSeg {
			continue
		}

		fullParams := max(dist.numDistinct-1, 1)
		leftParams := max(left.numDistinct-1, 1)
		rightParams := max(right.numDistinct-1, 1)

		penaltyFull := 0.5 * float64(fullParams) * math.Log(float64(n))
		penaltyLeft := 0.5 * float64(leftParams) * math.Log(float64(i))
		penaltyRight := 0.5 * float64(rightParams) * math.Log(float64(n-i))

		baseGain := costFull - (left.cost() + right.cost())
		penaltyDiff := (penaltyLeft + penaltyRight) - penaltyFull
		penalizedGain := baseGain - penaltyDiff

		if penalizedGain > maxGain {
			maxGain = penalizedGain
			bestK = i
		}
	}

	return maxGain > 0, bestK, maxGain
}

/*
LastGain caps reported gain at 1 for brightness-style hooks after a committed cut.
*/
func (mdl *MDL) LastGain() float64 {
	if mdl.lastGain > 1.0 {
		return 1.0
	}

	return mdl.lastGain
}

/*
symbolHistogram holds multiset counts for MDL coding cost on the active segment.
*/
type symbolHistogram struct {
	counts     map[uint32]int
	total      int
	numDistinct int
}

/*
newSymbolHistogram allocates empty counts for sliding-window split evaluation.
*/
func newSymbolHistogram() *symbolHistogram {
	return &symbolHistogram{counts: make(map[uint32]int)}
}

/*
add increments counts when a symbol enters the segment under test.
*/
func (hist *symbolHistogram) add(id uint32) {
	if hist.counts[id] == 0 {
		hist.numDistinct++
	}

	hist.counts[id]++
	hist.total++
}

/*
remove slides the split point right during boundary search.
*/
func (hist *symbolHistogram) remove(id uint32) {
	if hist.counts[id] <= 0 {
		return
	}

	hist.counts[id]--
	hist.total--

	if hist.counts[id] == 0 {
		delete(hist.counts, id)
		hist.numDistinct--
	}
}

/*
cost is Shannon code length in nats — lower means the segment compresses better.
*/
func (hist *symbolHistogram) cost() float64 {
	if hist.total == 0 {
		return 0
	}

	cost := 0.0
	n := float64(hist.total)

	for _, count := range hist.counts {
		if count == 0 {
			continue
		}

		p := float64(count) / n
		cost -= float64(count) * math.Log(p)
	}

	return cost
}

/*
clone snapshots the right segment while the left grows in the O(n) boundary scan.
*/
func (hist *symbolHistogram) clone() *symbolHistogram {
	out := newSymbolHistogram()
	out.total = hist.total
	out.numDistinct = hist.numDistinct

	for id, count := range hist.counts {
		out.counts[id] = count
	}

	return out
}
