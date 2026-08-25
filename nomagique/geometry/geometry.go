/*
Package geometry provides phase-fingerprint primitives: a high-dimensional
complex PhaseDial, an evenly spaced angular PhasePath, and a bounded, outcome-
tagged Corpus of retained dials.

A PhaseDial is a data primitive — a []complex128 of rotational phase gradients —
not a numeric scalar, so it is a concrete value type rather than a
types.Primitive. The corpus is a bounded ring whose entries are dials tagged
with the outcome that followed; it answers top-K signed-interference scans
against a query dial. The math is pure (Overlap/copy/normalize/rank); only the
corpus's retained entries need mutual exclusion for concurrent reads and writes.
*/
package geometry

import (
	"fmt"
	"math"
	"math/cmplx"
	"sort"
	"sync"
	"time"
)

/*
PhaseDial is a high-dimensional complex vector of rotational phase gradients.
Each component is a complex amplitude; its magnitude is scale and its argument
is phase. It carries no encoding policy — callers project their source (e.g. an
oscillator lattice) into it directly.
*/
type PhaseDial []complex128

/*
NewPhaseDial allocates a zero-initialized dial of the requested length.
*/
func NewPhaseDial(dimensions int) PhaseDial {
	return make(PhaseDial, dimensions)
}

/*
norm returns the L2 magnitude of the dial.
*/
func (dial PhaseDial) norm() float64 {
	var total float64

	for _, value := range dial {
		re, im := real(value), imag(value)
		total += re*re + im*im
	}

	return math.Sqrt(total)
}

/*
CopyAndNormalize returns a cloned, unit-normalized copy of the dial. An empty
or zero-energy dial is returned unchanged.
*/
func (dial PhaseDial) CopyAndNormalize() PhaseDial {
	out := make(PhaseDial, len(dial))
	copy(out, dial)

	return out.normalize()
}

func (dial PhaseDial) normalize() PhaseDial {
	var sumSq float64

	for _, value := range dial {
		re, im := real(value), imag(value)
		sumSq += re*re + im*im
	}

	if sumSq == 0 {
		return dial
	}

	inv := 1.0 / math.Sqrt(sumSq)

	for index := range dial {
		dial[index] = complex(real(dial[index])*inv, imag(dial[index])*inv)
	}

	return dial
}

/*
Overlap returns the normalized Hermitian inner product of two dials. Its
magnitude is their rotationally invariant affinity and its argument is the global
phase displacement that aligns them. Mismatched or empty dials yield zero.
*/
func (dial PhaseDial) Overlap(other PhaseDial) complex128 {
	if len(dial) != len(other) || len(dial) == 0 {
		return 0
	}

	var dot complex128
	var normA, normB float64

	for index := range dial {
		dot += cmplx.Conj(dial[index]) * other[index]
		reA, imA := real(dial[index]), imag(dial[index])
		reB, imB := real(other[index]), imag(other[index])
		normA += reA*reA + imA*imA
		normB += reB*reB + imB*imB
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / complex(math.Sqrt(normA)*math.Sqrt(normB), 0)
}

/*
PhasePath returns an evenly spaced angular path over the full circle, excluding
the endpoint so the last sample does not repeat the first. A scan is only
comparable between queries when every query is evaluated on the same path, so
the path is constructed here rather than by each caller.
*/
func PhasePath(samples int) ([]float64, error) {
	if samples <= 0 {
		return nil, fmt.Errorf("geometry: phase path requires a positive sample count")
	}

	angles := make([]float64, samples)

	for index := range angles {
		angles[index] = 2 * math.Pi * float64(index) / float64(samples)
	}

	return angles, nil
}

/*
CorpusEntry is one retained state snapshot tagged with the outcome that
followed. The dial is normalized at insert time so retrieval is pure similarity
arithmetic with no re-encoding.
*/
type CorpusEntry[Outcome any] struct {
	Dial    PhaseDial
	Outcome Outcome
	At      time.Time
}

/*
CorpusMatch is a single ranked result from a corpus similarity scan.
*/
type CorpusMatch[Outcome any] struct {
	Outcome    Outcome
	At         time.Time
	Similarity float64
}

/*
Corpus is a bounded, outcome-tagged collection of normalized dials. It answers
top-K signed-interference retrieval and controlled global phase scans. It is
safe for concurrent reads and writes.
*/
type Corpus[Outcome any] struct {
	mu         sync.RWMutex
	entries    []CorpusEntry[Outcome]
	maxSize    int
	dimensions int
	next       int
}

/*
NewCorpus creates a corpus with maximum capacity; when full, the oldest entries
are evicted to make room.
*/
func NewCorpus[Outcome any](maxSize int) (*Corpus[Outcome], error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("geometry: corpus capacity must be positive")
	}

	return &Corpus[Outcome]{
		entries: make([]CorpusEntry[Outcome], 0, maxSize),
		maxSize: maxSize,
	}, nil
}

/*
Insert adds one observation. At capacity the oldest entry is evicted. The dial
is normalized and copied so callers cannot mutate it after insertion.
*/
func (corpus *Corpus[Outcome]) Insert(entry CorpusEntry[Outcome]) error {
	if err := corpus.validate(entry.Dial); err != nil {
		return fmt.Errorf("geometry: insert corpus entry: %w", err)
	}

	entry.Dial = entry.Dial.CopyAndNormalize()

	corpus.mu.Lock()
	defer corpus.mu.Unlock()

	if corpus.dimensions == 0 {
		corpus.dimensions = len(entry.Dial)
	}

	if len(entry.Dial) != corpus.dimensions {
		return fmt.Errorf(
			"geometry: corpus dial has %d dimensions, expected %d",
			len(entry.Dial), corpus.dimensions,
		)
	}

	if len(corpus.entries) < corpus.maxSize {
		corpus.entries = append(corpus.entries, entry)

		return nil
	}

	corpus.entries[corpus.next] = entry
	corpus.next = (corpus.next + 1) % corpus.maxSize

	return nil
}

/*
Size returns the current number of retained entries.
*/
func (corpus *Corpus[Outcome]) Size() int {
	corpus.mu.RLock()
	defer corpus.mu.RUnlock()

	return len(corpus.entries)
}

/*
ScanPhases evaluates the corpus at each requested global phase rotation. The
complex overlaps are computed once, then analytically rotated, preserving both
constructive and destructive interference without reallocating rotated
fingerprints.
*/
func (corpus *Corpus[Outcome]) ScanPhases(
	queryDial PhaseDial, angles []float64, topK int,
) ([][]CorpusMatch[Outcome], error) {
	return corpus.scanPhases(queryDial, angles, topK, nil)
}

/*
ScanPhasesExcluding evaluates a controlled phase path while excluding entries at
the supplied timestamps, so a resident query cannot select itself.
*/
func (corpus *Corpus[Outcome]) ScanPhasesExcluding(
	queryDial PhaseDial,
	angles []float64,
	topK int,
	excludeTimes ...time.Time,
) ([][]CorpusMatch[Outcome], error) {
	excluded := make(map[int64]bool, len(excludeTimes))

	for _, excludeTime := range excludeTimes {
		excluded[excludeTime.UnixNano()] = true
	}

	return corpus.scanPhases(queryDial, angles, topK, excluded)
}

func (corpus *Corpus[Outcome]) scanPhases(
	queryDial PhaseDial, angles []float64, topK int, excluded map[int64]bool,
) ([][]CorpusMatch[Outcome], error) {
	if err := corpus.validate(queryDial); err != nil {
		return nil, fmt.Errorf("geometry: scan corpus: %w", err)
	}

	if topK <= 0 {
		return nil, fmt.Errorf("geometry: scan count must be positive")
	}

	if len(angles) == 0 {
		return nil, fmt.Errorf("geometry: phase scan requires at least one angle")
	}

	for _, angle := range angles {
		if math.IsNaN(angle) || math.IsInf(angle, 0) {
			return nil, fmt.Errorf("geometry: phase scan angle must be finite")
		}
	}

	corpus.mu.RLock()

	if corpus.dimensions != 0 && len(queryDial) != corpus.dimensions {
		corpus.mu.RUnlock()

		return nil, fmt.Errorf(
			"geometry: query dial has %d dimensions, expected %d",
			len(queryDial), corpus.dimensions,
		)
	}

	entries := make([]CorpusEntry[Outcome], 0, len(corpus.entries))
	overlaps := make([]complex128, 0, len(corpus.entries))

	for _, entry := range corpus.entries {
		if excluded[entry.At.UnixNano()] {
			continue
		}

		entries = append(entries, entry)
		overlaps = append(overlaps, queryDial.Overlap(entry.Dial))
	}

	corpus.mu.RUnlock()

	responses := make([][]CorpusMatch[Outcome], len(angles))
	matches := make([]CorpusMatch[Outcome], len(entries))

	for angleIndex, angle := range angles {
		rotation := cmplx.Rect(1, -angle)

		for entryIndex, entry := range entries {
			matches[entryIndex] = CorpusMatch[Outcome]{
				Outcome:    entry.Outcome,
				At:         entry.At,
				Similarity: real(overlaps[entryIndex] * rotation),
			}
		}

		corpus.rank(matches)
		responses[angleIndex] = append(
			[]CorpusMatch[Outcome](nil),
			matches[:min(topK, len(matches))]...,
		)
	}

	return responses, nil
}

func (corpus *Corpus[Outcome]) validate(dial PhaseDial) error {
	if len(dial) == 0 || dial.norm() == 0 {
		return fmt.Errorf("phase dial must contain nonzero amplitude")
	}

	for _, component := range dial {
		if math.IsNaN(real(component)) || math.IsNaN(imag(component)) ||
			math.IsInf(real(component), 0) || math.IsInf(imag(component), 0) {
			return fmt.Errorf("phase dial must contain finite components")
		}
	}

	return nil
}

func (corpus *Corpus[Outcome]) rank(matches []CorpusMatch[Outcome]) {
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].Similarity != matches[right].Similarity {
			return matches[left].Similarity > matches[right].Similarity
		}

		return matches[left].At.Before(matches[right].At)
	})
}
