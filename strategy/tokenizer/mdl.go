package tokenizer

import (
	"math"
	"math/bits"
)

/*
MDL discovers natural boundaries in a raw byte stream using minimum
description length, with optional calibrator tuning for split sensitivity.
*/
type MDL struct {
	calibrator *Calibrator
	eigen      *EigenMode

	buf  []byte
	dist *Distribution

	runningHist *Distribution

	prevSegLen  int
	fluxEmitted bool

	lastByteVal  float64
	lastEigenMag float64

	emaPhase float64
	emaPop   float64

	runningMeta SegmentMeta

	candidates []candidate
	offset     int

	MinSegmentBytes int

	ShannonCeiling float64

	PhaseThreshold float64
}

type candidate struct {
	k       int
	gain    float64
	entropy float64
	forced  bool
}

func NewMDL(calibrator *Calibrator) *MDL {
	minSeg := max(int(math.Log2(float64(VocabSize))/2), 2)

	return &MDL{
		calibrator:      calibrator,
		eigen:           NewEigenMode(),
		dist:            NewDistribution(),
		runningHist:     NewDistribution(),
		MinSegmentBytes: minSeg,
		ShannonCeiling:  ShannonCapacity,
		PhaseThreshold:  math.Pi / 2.0,
	}
}

func (seq *MDL) CloneEmpty() *MDL {
	return &MDL{
		calibrator:      seq.calibrator,
		eigen:           seq.eigen,
		dist:            NewDistribution(),
		runningHist:     NewDistribution(),
		MinSegmentBytes: seq.MinSegmentBytes,
		ShannonCeiling:  seq.ShannonCeiling,
		PhaseThreshold:  seq.PhaseThreshold,
	}
}

func (seq *MDL) Clone() *MDL {
	c := seq.CloneEmpty()
	c.buf = append([]byte(nil), seq.buf...)
	c.dist = seq.dist.Clone()
	c.runningHist = seq.runningHist.Clone()
	c.prevSegLen = seq.prevSegLen
	c.fluxEmitted = seq.fluxEmitted
	c.lastByteVal = seq.lastByteVal
	c.lastEigenMag = seq.lastEigenMag
	c.emaPhase = seq.emaPhase
	c.emaPop = seq.emaPop
	c.runningMeta = seq.runningMeta
	c.candidates = append([]candidate(nil), seq.candidates...)
	c.offset = seq.offset

	return c
}

func (seq *MDL) Analyze(pos uint32, byteVal byte) (bool, int, []int, SegmentMeta) {
	val, delta, eigenMag := seq.computeSignal(byteVal)

	seq.buf = append(seq.buf, byteVal)
	seq.dist.Add(byteVal)
	seq.runningHist.Add(byteVal)

	seq.runningMeta = seq.runningMeta.RollLeft(1)

	densityCeiling := seq.ShannonCeiling
	phaseThreshold := seq.PhaseThreshold

	if seq.calibrator != nil {
		densityCeiling = seq.calibrator.DensityCeiling(seq.ShannonCeiling)
		phaseThreshold = seq.calibrator.PhaseLimit(seq.PhaseThreshold)
	}

	canSplit := (len(seq.buf) - seq.offset) >= seq.MinSegmentBytes

	relativeOffset := len(seq.buf) - seq.offset - 1
	newBits := 0

	if relativeOffset > 0 {
		newBits = bits.OnesCount8(byteVal ^ seq.buf[len(seq.buf)-2])
	}

	shannonForced := seq.runningHist.ShannonDensity() > densityCeiling && canSplit
	phaseForced := seq.eigen != nil && seq.eigen.Trained && delta > phaseThreshold && canSplit
	primeForced := newBits >= 3 && canSplit

	isBoundary := false
	k := 0
	gain := 0.0

	if shannonForced || phaseForced || primeForced {
		isBoundary = true
		k = len(seq.buf) - seq.offset
		gain = 1.0
	} else if canSplit {
		mdlBoundary, mdlK, mdlGain := seq.detectBoundary(seq.buf[seq.offset:], seq.dist)
		if mdlBoundary {
			isBoundary = true
			k = mdlK
			gain = mdlGain
		}
	}

	var events []int

	emitK := 0

	if isBoundary {
		absK := seq.offset + k
		seq.candidates = append(seq.candidates, candidate{k: absK, gain: gain, forced: shannonForced || phaseForced || primeForced})
		seq.offset = absK

		seq.dist = NewDistribution()

		for _, b := range seq.buf[seq.offset:] {
			seq.dist.Add(b)
		}

		seq.runningHist = NewDistribution()
	}

	if seq.calibrator != nil && seq.eigen != nil && seq.eigen.Trained {
		seq.calibrator.ObservePhase(delta)
	}

	if len(seq.candidates) >= 2 {
		seq.balanceCandidates()
	}

	if len(seq.candidates) >= 2 {
		emitK = seq.candidates[0].k

		events = append(events, seq.classifyDirection(seq.buf, emitK))
		events = append(events, EventPhaseInversion)

		for _, ev := range events {
			seq.runningMeta = tagMeta(seq.runningMeta, ev)
		}

		emitMeta := seq.runningMeta

		emitDist := seq.getDistribution(0, emitK)
		emitDensity := emitDist.ShannonDensity()

		maxBits := float64(emitK * 8)
		actualBits := float64(emitDist.NumDistinct() * 8)

		primeCoherence := 0.0

		if maxBits > 0 {
			primeCoherence = math.Max(0, 1.0-(actualBits/maxBits))
		}

		phaseCoherence := math.Exp(-seq.emaPhase)

		seq.emitSplit(emitK)

		seq.candidates = seq.candidates[1:]

		for i := range seq.candidates {
			seq.candidates[i].k -= emitK
		}

		seq.offset -= emitK
		seq.updateEMA(val, delta, eigenMag)

		if seq.calibrator != nil {
			seq.calibrator.FeedbackChunk(emitDensity, primeCoherence, phaseCoherence)
		}

		return true, emitK, events, emitMeta
	}

	if seq.hasFlux() {
		events = append(events, EventLowVarianceFlux)
		seq.fluxEmitted = true
		seq.runningMeta = tagMeta(seq.runningMeta, EventLowVarianceFlux)
	}

	seq.updateEMA(val, delta, eigenMag)

	return false, emitK, events, seq.runningMeta
}

func (seq *MDL) hasFlux() bool {
	return seq.prevSegLen > 0 && (len(seq.buf)-seq.offset) >= seq.prevSegLen && !seq.fluxEmitted
}

func (seq *MDL) computeSignal(byteVal byte) (val, delta, eigenMag float64) {
	val = float64(byteVal)

	if seq.eigen != nil && seq.eigen.Trained {
		theta, phi := seq.eigen.PhaseForByte(byteVal)
		eigenMag = math.Hypot(theta, phi)
		delta = math.Abs(eigenMag - seq.lastEigenMag)

		return val, delta, eigenMag
	}

	delta = math.Abs(val - seq.lastByteVal)

	return val, delta, eigenMag
}

func (seq *MDL) detectBoundary(buf []byte, dist *Distribution) (bool, int, float64) {
	n := len(buf)
	minSeg := max(seq.MinSegmentBytes, 2)

	if n < 2*minSeg {
		return false, 0, 0
	}

	penaltyScale := 1.0

	if seq.calibrator != nil && seq.calibrator.SensitivityPop() > 0 {
		penaltyScale = seq.calibrator.SensitivityPop()
	}

	bestK := 0
	maxGain := 0.0

	costFull := dist.Cost()
	left := NewDistribution()
	right := dist.Clone()

	for i := 1; i < n; i++ {
		b := buf[i-1]

		left.Add(b)
		right.Remove(b)

		if i < minSeg || n-i < minSeg {
			continue
		}

		fullParams := max(dist.NumDistinct()-1, 1)
		leftParams := max(left.NumDistinct()-1, 1)
		rightParams := max(right.NumDistinct()-1, 1)

		penaltyFull := 0.5 * float64(fullParams) * math.Log(float64(n))
		penaltyLeft := 0.5 * float64(leftParams) * math.Log(float64(i))
		penaltyRight := 0.5 * float64(rightParams) * math.Log(float64(n-i))

		baseGain := costFull - (left.Cost() + right.Cost())
		penaltyDiff := (penaltyLeft + penaltyRight) - penaltyFull

		penalizedGain := baseGain - (penaltyScale * penaltyDiff)

		if penalizedGain > maxGain {
			maxGain = penalizedGain
			bestK = i
		}
	}

	return maxGain > 0, bestK, maxGain
}

func (seq *MDL) emitSplit(k int) {
	seq.prevSegLen = k
	seq.buf = append([]byte(nil), seq.buf[k:]...)
	seq.fluxEmitted = false
}

func (seq *MDL) balanceCandidates() {
	if len(seq.candidates) < 2 {
		return
	}

	if seq.candidates[0].forced || seq.candidates[1].forced {
		return
	}

	c1 := &seq.candidates[0]
	c2 := &seq.candidates[1]

	combinedBuf := seq.buf[:c2.k]
	jointDist := seq.getDistribution(0, c2.k)

	found, bestK, gain := seq.detectBoundary(combinedBuf, jointDist)

	if !found {
		seq.candidates = seq.candidates[1:]
		return
	}

	d1 := seq.getDistribution(0, bestK)
	d2 := seq.getDistribution(bestK, c2.k)

	if seq.isSimilar(d1, d2) {
		seq.candidates = seq.candidates[1:]
		return
	}

	c1.k = bestK
	c1.gain = gain
}

func (seq *MDL) getDistribution(start, end int) *Distribution {
	dist := NewDistribution()

	for _, b := range seq.buf[start:end] {
		dist.Add(b)
	}

	return dist
}

func (seq *MDL) isSimilar(d1, d2 *Distribution) bool {
	if d1.N() == 0 || d2.N() == 0 {
		return false
	}

	costSplit := d1.Cost() + d2.Cost()
	dCombined := d1.Clone()
	dCombined.AddFrom(d2)
	costCombined := dCombined.Cost()

	return costCombined <= costSplit
}

func (seq *MDL) classifyDirection(buf []byte, k int) int {
	if k <= 0 || k >= len(buf) {
		return EventDensityTrough
	}

	var leftSum, rightSum int

	for _, b := range buf[:k] {
		leftSum += int(b)
	}

	for _, b := range buf[k:] {
		rightSum += int(b)
	}

	leftMean := float64(leftSum) / float64(k)
	rightMean := float64(rightSum) / float64(len(buf)-k)

	if rightMean > leftMean {
		return EventDensitySpike
	}

	return EventDensityTrough
}

func (seq *MDL) updateEMA(val, delta, eigenMag float64) {
	seq.lastByteVal = val

	if seq.eigen != nil && seq.eigen.Trained {
		seq.lastEigenMag = eigenMag
	}

	n := max(len(seq.buf), 1)
	alpha := 1.0 / float64(n)
	seq.emaPop = seq.emaPop*(1-alpha) + val*alpha
	seq.emaPhase = seq.emaPhase*(1-alpha) + delta*alpha
}

func (seq *MDL) Forecast(pos int, byteVal byte) (bool, []int, SegmentMeta) {
	buf := append(seq.buf, byteVal)
	dist := seq.dist.Clone()
	dist.Add(byteVal)

	window := buf[seq.offset:]
	isBoundary, k, _ := seq.detectBoundary(window, dist)

	if !isBoundary {
		return false, nil, seq.runningMeta
	}

	events := append([]int{}, seq.classifyDirection(window, k))
	events = append(events, EventPhaseInversion)

	tempMeta := seq.runningMeta.RollLeft(1)

	for _, ev := range events {
		tempMeta = tagMeta(tempMeta, ev)
	}

	return true, events, tempMeta
}

func (seq *MDL) FeedbackRetrievalQuality(overDiscriminated, underDiscriminated bool) {
	if seq.calibrator == nil {
		return
	}

	_, stddev := seq.calibrator.WindowStats()
	adjust := stddev

	if overDiscriminated {
		seq.calibrator.SetSensitivityPop(seq.calibrator.SensitivityPop() * math.Exp(adjust))
	} else if underDiscriminated {
		seq.calibrator.SetSensitivityPop(seq.calibrator.SensitivityPop() * math.Exp(-adjust))
	}
}

func (seq *MDL) Phase() (float64, float64) {
	return seq.emaPhase, seq.dist.Cost() / float64(max(seq.dist.N(), 1))
}

func (seq *MDL) SetEigenMode(eigen *EigenMode) {
	if eigen == nil {
		seq.eigen = NewEigenMode()
		return
	}

	seq.eigen = eigen
}

func (seq *MDL) Flush() (bool, int, []int, SegmentMeta) {
	if len(seq.candidates) == 0 {
		return false, 0, nil, seq.runningMeta
	}

	emitK := seq.candidates[0].k
	events := append([]int{}, seq.classifyDirection(seq.buf, emitK))
	events = append(events, EventPhaseInversion)

	for _, ev := range events {
		seq.runningMeta = tagMeta(seq.runningMeta, ev)
	}

	emitMeta := seq.runningMeta

	seq.emitSplit(emitK)

	seq.candidates = seq.candidates[1:]

	for i := range seq.candidates {
		seq.candidates[i].k -= emitK
	}

	seq.offset -= emitK

	return true, emitK, events, emitMeta
}
