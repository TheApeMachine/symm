package hindsight

import (
	"sort"
)

/*
Verifier scans a Tape and reports its capture-integrity state (§47) as a set of
concrete Gap findings. Certainty fails closed: any gap marks the tape GAPPED,
and any corruption marks it CORRUPT — replay and inspection must never treat an
incomplete tape as complete evidence (§48).

The verifier reasons about identity and provenance only. It never correlates
records by timestamp, never sorts by venue time, and never guesses a missing
relationship (§9, §41).
*/
type Verifier struct {
	gaps    []Gap
	corrupt bool
}

/*
NewVerifier builds an empty verifier.
*/
func NewVerifier() *Verifier {
	return &Verifier{}
}

/*
Verify scans the tape and returns its integrity verdict plus every Gap found.
*/
func (verifier *Verifier) Verify(tape *Tape) (Integrity, []Gap) {
	if verifier == nil {
		return IntegrityComplete, nil
	}

	verifier.gaps = verifier.gaps[:0]
	verifier.corrupt = false

	if tape == nil {
		return IntegrityComplete, nil
	}

	verifier.verifySequences(tape)
	verifier.verifyFrames(tape)
	verifier.verifyEpochs(tape)
	verifier.verifyManifests(tape)
	verifier.verifyWitnesses(tape)

	if verifier.corrupt {
		return IntegrityCorrupt, append([]Gap(nil), verifier.gaps...)
	}

	if len(verifier.gaps) > 0 {
		return IntegrityGapped, append([]Gap(nil), verifier.gaps...)
	}

	return IntegrityComplete, nil
}

/*
verifySequences detects missing CaptureSequence values across the whole run.
CaptureSequence is the primary ordering (§6); a hole means an observation was
not captured, so inspection across that interval is broken (§48).
*/
func (verifier *Verifier) verifySequences(tape *Tape) {
	sequences := make([]CaptureSequence, 0, len(tape.framesBySeq))

	for sequence := range tape.framesBySeq {
		sequences = append(sequences, sequence)
	}

	if len(sequences) == 0 {
		return
	}

	sort.Slice(sequences, func(left, right int) bool {
		return sequences[left] < sequences[right]
	})

	minSequence := sequences[0]

	for sequence := minSequence; sequence < sequences[len(sequences)-1]; sequence++ {
		if _, exists := tape.framesBySeq[sequence]; exists {
			continue
		}

		verifier.gaps = append(verifier.gaps, Gap{
			Encoding: GapMissingSequence,
			Sequence: sequence,
		})
	}
}

/*
verifyFrames detects payload and hash defects on raw frames. A missing payload
marks the tape gapped; a payload whose stored hash disagrees with its bytes
marks it corrupt (§10, §47).
*/
func (verifier *Verifier) verifyFrames(tape *Tape) {
	for identity, frame := range tape.frames {
		if len(frame.Payload) == 0 {
			verifier.gaps = append(verifier.gaps, Gap{
				Encoding: GapMissingPayload,
				Sequence: identity.Sequence,
			})

			continue
		}

		if frame.PayloadHash != hashPayload(frame.Payload) {
			verifier.gaps = append(verifier.gaps, Gap{
				Encoding: GapHashMismatch,
				Sequence: identity.Sequence,
			})
			verifier.corrupt = true
		}
	}
}

/*
verifyEpochs detects a broken stream epoch: for one stream, the epoch must never
decrease across capture order (§7). A reconnect bumps the epoch upward; reusing
the same stream identity with a lower epoch means the stream's epochs were
mangled, so inspection cannot disambiguate frames before and after a reconnect.
*/
func (verifier *Verifier) verifyEpochs(tape *Tape) {
	byStream := make(map[Stream][]CaptureIdentity)

	for identity := range tape.frames {
		byStream[identity.Stream] = append(byStream[identity.Stream], identity)
	}

	for _, identities := range byStream {
		// Capture sequence is the arrival order; the epoch must be monotonic
		// non-decreasing across it. Sorting by epoch would destroy the very
		// regression this check exists to find.
		sort.SliceStable(identities, func(left, right int) bool {
			return identities[left].Sequence < identities[right].Sequence
		})

		var (
			lastEpoch StreamEpoch
			first     = true
		)

		for _, identity := range identities {
			if first {
				lastEpoch = identity.StreamEpoch
				first = false

				continue
			}

			if identity.StreamEpoch < lastEpoch {
				verifier.corrupt = true
				verifier.gaps = append(verifier.gaps, Gap{
					Encoding: GapBrokenEpoch,
					Sequence: identity.Sequence,
				})
			}

			lastEpoch = identity.StreamEpoch
		}
	}
}

/*
verifyManifests detects envelope defects: an envelope whose origin has no raw
frame is a dangling reference (§47). Ordinal uniqueness needs no separate check
here — the tape keys manifests by EnvelopeRef, so two envelopes sharing one
origin and one ordinal are the same identity and the second write is rejected
outright rather than silently recorded.
*/
func (verifier *Verifier) verifyManifests(tape *Tape) {
	for ref := range tape.manifests {
		if _, exists := tape.frames[ref.Origin]; exists {
			continue
		}

		verifier.gaps = append(verifier.gaps, Gap{
			Encoding: GapDanglingEnvelope,
			Sequence: ref.Origin.Sequence,
		})
	}
}

/*
verifyWitnesses detects witness defects: a witness whose envelope has no
manifest is a dangling reference (§47), and a witness whose immediate parents
reference an envelope this tape has never recorded breaks provenance (§23).
*/
func (verifier *Verifier) verifyWitnesses(tape *Tape) {
	for ref, witnesses := range tape.witnesses {
		if _, exists := tape.manifests[ref]; !exists {
			verifier.gaps = append(verifier.gaps, Gap{
				Encoding: GapDanglingWitness,
				Sequence: ref.Origin.Sequence,
			})
		}

		for _, witness := range witnesses {
			for _, parent := range witness.ImmediateParents {
				if _, exists := tape.manifests[parent]; exists {
					continue
				}

				verifier.gaps = append(verifier.gaps, Gap{
					Encoding: GapMalformedProvenance,
					Sequence: ref.Origin.Sequence,
				})
			}
		}
	}
}
