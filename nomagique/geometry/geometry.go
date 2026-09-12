/*
Package geometry provides phase-fingerprint primitives: a high-dimensional
complex PhaseDial, an evenly spaced angular PhasePath, Hermitian Overlap, and a
bounded, outcome-tagged Corpus of retained dials.

Everything is a streaming Primitive over an unsafe.Pointer wire. Payloads are
plain data types: PhaseDial is a []complex128 of rotational phase gradients,
and every command, query, and reading is a struct with exported fields only.

The math is pure (dial overlap/copy/normalize/rank); only the corpus's retained
entries need mutual exclusion for concurrent reads and writes, and the Corpus
primitive owns that mutex itself.
*/
package geometry

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"math/cmplx"
	"sort"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
PhaseDial is a high-dimensional complex vector of rotational phase gradients.
Each component is a complex amplitude; its magnitude is scale and its argument
is phase. It carries no encoding policy — callers project their source (e.g. an
oscillator lattice) into it directly. It is pure wire payload: no methods.
*/
type PhaseDial []complex128

/*
dialNorm returns the L2 magnitude of the dial.
*/
func dialNorm(dial PhaseDial) float64 {
	var total float64

	for _, value := range dial {
		re, im := real(value), imag(value)
		total += re*re + im*im
	}

	return math.Sqrt(total)
}

/*
normalizeDial scales the dial to unit energy in place. An empty or zero-energy
dial is returned unchanged.
*/
func normalizeDial(dial PhaseDial) PhaseDial {
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
copyAndNormalize returns a cloned, unit-normalized copy of the dial so callers
cannot mutate a retained fingerprint through their original reference.
*/
func copyAndNormalize(dial PhaseDial) PhaseDial {
	out := make(PhaseDial, len(dial))
	copy(out, dial)

	return normalizeDial(out)
}

/*
dialOverlap returns the normalized Hermitian inner product of two dials. Its
magnitude is their rotationally invariant affinity and its argument is the
global phase displacement that aligns them. Mismatched or empty dials yield
zero.
*/
func dialOverlap(dial, other PhaseDial) complex128 {
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
validateDial rejects empty, zero-energy, or non-finite dials.
*/
func validateDial(dial PhaseDial) error {
	if len(dial) == 0 || dialNorm(dial) == 0 {
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

/*
PhasePathReading is the evenly spaced angular path over the full circle,
excluding the endpoint so the last sample does not repeat the first.
*/
type PhasePathReading struct {
	Angles []float64
}

/*
PhasePath owns angular path construction. A scan is only comparable between
queries when every query is evaluated on the same path, so the path is
constructed here rather than by each caller.
*/
type PhasePath struct {
	err error
	out PhasePathReading
}

/*
NewPhasePath creates a PhasePath primitive.
*/
func NewPhasePath() core.Primitive {
	return &PhasePath{}
}

/*
Next receives *int sample counts and yields a *PhasePathReading with the
evenly spaced angles. A non-positive count is a domain failure: it is recorded
and the stream ends.
*/
func (op *PhasePath) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			samples := *(*int)(arriving)

			if samples <= 0 {
				op.Error(fmt.Errorf(
					"%w: geometry: phase path requires a positive sample count",
					core.ErrDomain,
				))
				return
			}

			angles := make([]float64, samples)

			for index := range angles {
				angles[index] = 2 * math.Pi * float64(index) / float64(samples)
			}

			op.out = PhasePathReading{Angles: angles}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *PhasePath) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Normalize owns unit-energy scaling of arriving dials.
*/
type Normalize struct {
	err error
}

/*
NewNormalize creates a Normalize primitive.
*/
func NewNormalize() core.Primitive {
	return &Normalize{}
}

/*
Next receives *PhaseDial pointers, normalizes each dial in place, and yields
the same pointer. An empty or zero-energy dial passes through unchanged.
*/
func (op *Normalize) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			dial := (*PhaseDial)(arriving)
			*dial = normalizeDial(*dial)

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Normalize) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
OverlapPair is the two-dial wire payload of the Overlap primitive.
*/
type OverlapPair struct {
	Probe PhaseDial
	Entry PhaseDial
}

/*
Overlap owns the normalized Hermitian inner product between two dials.
*/
type Overlap struct {
	err error
	out complex128
}

/*
NewOverlap creates an Overlap primitive.
*/
func NewOverlap() core.Primitive {
	return &Overlap{}
}

/*
Next receives *OverlapPair payloads and yields a *complex128 normalized
Hermitian overlap. Mismatched or empty pairs yield zero, matching the pure
dial math.
*/
func (op *Overlap) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			pair := (*OverlapPair)(arriving)
			op.out = dialOverlap(pair.Probe, pair.Entry)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Overlap) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
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
CorpusQuery asks for a controlled phase scan: the query dial, the global phase
angles to evaluate, the top-K count, and the entry timestamps to exclude so a
resident query cannot select itself.
*/
type CorpusQuery struct {
	Dial         PhaseDial
	Angles       []float64
	TopK         int
	ExcludeTimes []time.Time
}

/*
CorpusCount asks for the number of retained entries.
*/
type CorpusCount struct{}

/*
CorpusCommand discriminates one corpus operation. Exactly one of Insert, Query,
or Count must be set; anything else is a shape failure.
*/
type CorpusCommand[Outcome any] struct {
	Insert *CorpusEntry[Outcome]
	Query  *CorpusQuery
	Count  *CorpusCount
}

/*
CorpusResult is one acknowledgement or scan response: an insertion
acknowledgement, a retained-entry count, or the top-K matches for every
requested angle.
*/
type CorpusResult[Outcome any] struct {
	Inserted bool
	Size     int
	Scan     [][]CorpusMatch[Outcome]
}

/*
Corpus is a bounded, outcome-tagged collection of normalized dials, owned by
one Primitive. It answers top-K signed-interference retrieval and controlled
global phase scans. It is safe for concurrent reads and writes: the primitive
owns the mutex around its retained entries.
*/
type Corpus[Outcome any] struct {
	err        error
	mu         sync.RWMutex
	entries    []CorpusEntry[Outcome]
	maxSize    int
	dimensions int
	next       int
	out        CorpusResult[Outcome]
}

/*
NewCorpus creates a corpus primitive with maximum capacity; when full, the
oldest entries are evicted to make room. A non-positive capacity is recorded
as a domain failure and every stream over the primitive yields nothing.
*/
func NewCorpus[Outcome any](maxSize int) core.Primitive {
	if maxSize <= 0 {
		return &Corpus[Outcome]{
			err: fmt.Errorf("%w: geometry: corpus capacity must be positive", core.ErrDomain),
		}
	}

	return &Corpus[Outcome]{
		entries: make([]CorpusEntry[Outcome], 0, maxSize),
		maxSize: maxSize,
	}
}

/*
Next receives *CorpusCommand payloads and yields a *CorpusResult for each:
insertion acknowledgements, retained-entry counts, or per-angle top-K scan
rows. Any invalid command ends the stream with the error recorded.
*/
func (op *Corpus[Outcome]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*CorpusCommand[Outcome])(arriving)
			result, err := op.execute(command)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Corpus[Outcome]) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
execute dispatches one command to its intent and returns its result.
*/
func (op *Corpus[Outcome]) execute(
	command *CorpusCommand[Outcome],
) (CorpusResult[Outcome], error) {
	intents := 0

	if command.Insert != nil {
		intents++
	}

	if command.Query != nil {
		intents++
	}

	if command.Count != nil {
		intents++
	}

	if intents != 1 {
		return CorpusResult[Outcome]{}, fmt.Errorf(
			"%w: geometry: corpus command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.Insert != nil {
		return op.insert(*command.Insert)
	}

	if command.Query != nil {
		return op.scan(command.Query)
	}

	return CorpusResult[Outcome]{Size: op.size()}, nil
}

/*
insert adds one observation. At capacity the oldest entry is evicted. The dial
is normalized and copied so callers cannot mutate it after insertion.
*/
func (op *Corpus[Outcome]) insert(
	entry CorpusEntry[Outcome],
) (CorpusResult[Outcome], error) {
	if err := validateDial(entry.Dial); err != nil {
		return CorpusResult[Outcome]{}, fmt.Errorf("geometry: insert corpus entry: %w", err)
	}

	entry.Dial = copyAndNormalize(entry.Dial)

	op.mu.Lock()
	defer op.mu.Unlock()

	if op.dimensions == 0 {
		op.dimensions = len(entry.Dial)
	}

	if len(entry.Dial) != op.dimensions {
		return CorpusResult[Outcome]{}, fmt.Errorf(
			"%w: geometry: corpus dial has %d dimensions, expected %d",
			core.ErrShape, len(entry.Dial), op.dimensions,
		)
	}

	if len(op.entries) < op.maxSize {
		op.entries = append(op.entries, entry)

		return CorpusResult[Outcome]{Inserted: true}, nil
	}

	op.entries[op.next] = entry
	op.next = (op.next + 1) % op.maxSize

	return CorpusResult[Outcome]{Inserted: true}, nil
}

/*
size returns the current number of retained entries under the read lock.
*/
func (op *Corpus[Outcome]) size() int {
	op.mu.RLock()
	defer op.mu.RUnlock()

	return len(op.entries)
}

/*
scan evaluates the corpus at each requested global phase rotation. The complex
overlaps are computed once, then analytically rotated, preserving both
constructive and destructive interference without reallocating rotated
fingerprints. Entries at the excluded timestamps are skipped.
*/
func (op *Corpus[Outcome]) scan(
	query *CorpusQuery,
) (CorpusResult[Outcome], error) {
	if err := validateDial(query.Dial); err != nil {
		return CorpusResult[Outcome]{}, fmt.Errorf("geometry: scan corpus: %w", err)
	}

	if query.TopK <= 0 {
		return CorpusResult[Outcome]{}, fmt.Errorf(
			"%w: geometry: scan count must be positive", core.ErrDomain,
		)
	}

	if len(query.Angles) == 0 {
		return CorpusResult[Outcome]{}, fmt.Errorf(
			"%w: geometry: phase scan requires at least one angle", core.ErrDomain,
		)
	}

	for _, angle := range query.Angles {
		if math.IsNaN(angle) || math.IsInf(angle, 0) {
			return CorpusResult[Outcome]{}, fmt.Errorf(
				"%w: geometry: phase scan angle must be finite", core.ErrDomain,
			)
		}
	}

	excluded := make(map[int64]bool, len(query.ExcludeTimes))

	for _, excludeTime := range query.ExcludeTimes {
		excluded[excludeTime.UnixNano()] = true
	}

	op.mu.RLock()

	if op.dimensions != 0 && len(query.Dial) != op.dimensions {
		op.mu.RUnlock()

		return CorpusResult[Outcome]{}, fmt.Errorf(
			"%w: geometry: query dial has %d dimensions, expected %d",
			core.ErrShape, len(query.Dial), op.dimensions,
		)
	}

	entries := make([]CorpusEntry[Outcome], 0, len(op.entries))
	overlaps := make([]complex128, 0, len(op.entries))

	for _, entry := range op.entries {
		if excluded[entry.At.UnixNano()] {
			continue
		}

		entries = append(entries, entry)
		overlaps = append(overlaps, dialOverlap(query.Dial, entry.Dial))
	}

	op.mu.RUnlock()

	responses := make([][]CorpusMatch[Outcome], len(query.Angles))
	matches := make([]CorpusMatch[Outcome], len(entries))

	for angleIndex, angle := range query.Angles {
		rotation := cmplx.Rect(1, -angle)

		for entryIndex, entry := range entries {
			matches[entryIndex] = CorpusMatch[Outcome]{
				Outcome:    entry.Outcome,
				At:         entry.At,
				Similarity: real(overlaps[entryIndex] * rotation),
			}
		}

		rankMatches(matches)
		responses[angleIndex] = append(
			[]CorpusMatch[Outcome](nil),
			matches[:min(query.TopK, len(matches))]...,
		)
	}

	return CorpusResult[Outcome]{Scan: responses}, nil
}

/*
rankMatches orders matches by descending similarity, breaking ties by the
earlier timestamp.
*/
func rankMatches[Outcome any](matches []CorpusMatch[Outcome]) {
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].Similarity != matches[right].Similarity {
			return matches[left].Similarity > matches[right].Similarity
		}

		return matches[left].At.Before(matches[right].At)
	})
}
