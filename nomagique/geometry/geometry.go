/*
Package geometry provides phase-fingerprint primitives: a high-dimensional
complex PhaseDial, an evenly spaced angular PhasePath, Hermitian Overlap, and a
bounded, outcome-tagged Corpus of retained dials.

Everything is a types.Value closure over typed payloads. Payloads are
plain data types: PhaseDial is a []complex128 of rotational phase gradients,
and every command, query, and reading is a data struct.
*/
package geometry

import (
	"fmt"
	"math"
	"math/cmplx"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
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

	inv := core.Unit / math.Sqrt(sumSq)

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
NewPhasePath creates a PhasePath Value closure.
A non-positive count yields an empty reading.
*/
type PhasePath types.Value[int, PhasePathReading]
func NewPhasePath() PhasePath {
	return func(samples int) PhasePathReading {
		if samples <= 0 {
			return PhasePathReading{}
		}

		angles := make([]float64, samples)
		for index := range angles {
			angles[index] = 2 * math.Pi * float64(index) / float64(samples)
		}

		return PhasePathReading{Angles: angles}
	}
}

/*
NewNormalize creates a Normalize Value closure that normalizes arriving dials to unit energy.
An empty or zero-energy dial passes through unchanged.
*/
type Normalize types.Value[PhaseDial, PhaseDial]
func NewNormalize() Normalize {
	return func(dial PhaseDial) PhaseDial {
		return normalizeDial(dial)
	}
}

/*
OverlapPair is the two-dial data payload of the Overlap primitive.
*/
type OverlapPair struct {
	Probe PhaseDial
	Entry PhaseDial
}

/*
NewOverlap creates an Overlap Value closure.
Mismatched or empty pairs yield zero.
*/
type Overlap types.Value[OverlapPair, complex128]
func NewOverlap() Overlap {
	return func(pair OverlapPair) complex128 {
		return dialOverlap(pair.Probe, pair.Entry)
	}
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
or Count must be set; anything else is an invalid command.
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
NewCorpus creates a corpus Value closure with maximum capacity; when full, the
oldest entries are evicted to make room. State is encapsulated purely inside the closure.
*/
type Corpus[Outcome any] types.Value[CorpusCommand[Outcome], CorpusResult[Outcome]]
func NewCorpus[Outcome any](maxSize int) Corpus[Outcome] {
	if maxSize <= 0 {
		return func(CorpusCommand[Outcome]) CorpusResult[Outcome] {
			return CorpusResult[Outcome]{}
		}
	}

	var mu sync.RWMutex
	entries := make([]CorpusEntry[Outcome], 0, maxSize)
	dimensions := 0
	next := 0

	return func(command CorpusCommand[Outcome]) CorpusResult[Outcome] {
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
			return CorpusResult[Outcome]{}
		}

		if command.Insert != nil {
			entry := *command.Insert
			if err := validateDial(entry.Dial); err != nil {
				return CorpusResult[Outcome]{}
			}
			entry.Dial = copyAndNormalize(entry.Dial)

			mu.Lock()
			defer mu.Unlock()

			if dimensions == 0 {
				dimensions = len(entry.Dial)
			}
			if len(entry.Dial) != dimensions {
				return CorpusResult[Outcome]{}
			}

			if len(entries) < maxSize {
				entries = append(entries, entry)
				return CorpusResult[Outcome]{Inserted: true}
			}

			entries[next] = entry
			next = (next + 1) % maxSize
			return CorpusResult[Outcome]{Inserted: true}
		}

		if command.Query != nil {
			query := command.Query
			if err := validateDial(query.Dial); err != nil {
				return CorpusResult[Outcome]{}
			}
			if query.TopK <= 0 || len(query.Angles) == 0 {
				return CorpusResult[Outcome]{}
			}
			for _, angle := range query.Angles {
				if math.IsNaN(angle) || math.IsInf(angle, 0) {
					return CorpusResult[Outcome]{}
				}
			}

			excluded := make(map[int64]bool, len(query.ExcludeTimes))
			for _, excludeTime := range query.ExcludeTimes {
				excluded[excludeTime.UnixNano()] = true
			}

			mu.RLock()
			if dimensions != 0 && len(query.Dial) != dimensions {
				mu.RUnlock()
				return CorpusResult[Outcome]{}
			}

			evalEntries := make([]CorpusEntry[Outcome], 0, len(entries))
			overlaps := make([]complex128, 0, len(entries))
			for _, entry := range entries {
				if excluded[entry.At.UnixNano()] {
					continue
				}
				evalEntries = append(evalEntries, entry)
				overlaps = append(overlaps, dialOverlap(query.Dial, entry.Dial))
			}
			mu.RUnlock()

			responses := make([][]CorpusMatch[Outcome], len(query.Angles))
			matches := make([]CorpusMatch[Outcome], len(evalEntries))

			for angleIndex, angle := range query.Angles {
				rotation := cmplx.Rect(1, -angle)
				for entryIndex, entry := range evalEntries {
					matches[entryIndex] = CorpusMatch[Outcome]{
						Outcome:    entry.Outcome,
						At:         entry.At,
						Similarity: real(overlaps[entryIndex] * rotation),
					}
				}
				rankMatches(matches)
				limit := min(query.TopK, len(matches))
				responses[angleIndex] = append(
					[]CorpusMatch[Outcome](nil),
					matches[:limit]...,
				)
			}

			return CorpusResult[Outcome]{Scan: responses}
		}

		mu.RLock()
		sz := len(entries)
		mu.RUnlock()
		return CorpusResult[Outcome]{Size: sz}
	}
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
