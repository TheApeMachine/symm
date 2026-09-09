package strategy

import (
	"context"
	"sort"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
)

// readInputs admits original precursor measurements at their exact capture
// coordinate. A witness with no raw quote uses the last preceding captured
// touch; later quotes cannot fill a missing side. Old decisions are not read.
func (rehearsal *Rehearsal) readInputs(ctx context.Context) error {
	if rehearsal.inputs == nil {
		rehearsal.inputs = make(map[hindsight.EnvelopeRef]hindsight.RehearsalInput)
		rehearsal.witnessed = make(map[tables.EnvelopeRefRow]bool)
	}
	witnesses, err := rehearsal.catalog.WitnessesUnseen(ctx, string(rehearsal.run), "precursor", rehearsal.witnessed)

	if err != nil {
		return errnie.Error(err)
	}
	sort.SliceStable(witnesses, func(left, right int) bool {
		if witnesses[left].Envelope.Sequence != witnesses[right].Envelope.Sequence {
			return witnesses[left].Envelope.Sequence < witnesses[right].Envelope.Sequence
		}
		return witnesses[left].Envelope.Ordinal < witnesses[right].Envelope.Ordinal
	})

	sequences := make([]int64, 0, len(witnesses))
	for _, witness := range witnesses {
		sequences = append(sequences, witness.Envelope.Sequence)
	}
	captures, err := rehearsal.catalog.CaptureReferences(ctx, string(rehearsal.run), sequences)

	if err != nil {
		return errnie.Error(err)
	}

	for _, witness := range witnesses {
		if witness.Envelope.Sequence > rehearsal.lastSequence {
			continue // Its raw capture has not reached this replay pass yet.
		}
		if err := rehearsal.admitInput(witness, captures); err != nil {
			return errnie.Error(err)
		}
		rehearsal.witnessed[witness.Envelope] = true
	}
	return nil
}

func (rehearsal *Rehearsal) admitInput(row tables.WitnessRow, captures map[int64]tables.CaptureRow) error {
	capture, found := captures[row.Envelope.Sequence]

	if !found {
		return errnie.Error(errnie.Err(errnie.NotFound, "rehearsal: precursor has no captured input", nil))
	}
	reference := hindsight.EnvelopeRef{Origin: hindsight.FrameFromRow(capture).Identity, Ordinal: uint64(row.Envelope.Ordinal)}
	input, err := (hindsight.ArtifactWitness{Envelope: reference, Payload: row.Payload}).RehearsalInput()

	if err != nil {
		return errnie.Error(err)
	}

	if len(input.Measurements) == 0 {
		return nil // The captured envelope did not produce precursor measurements.
	}
	input.At = capture.ReceivedAt
	rehearsal.inputs[reference] = input
	symbols := make(map[string]struct{})

	for _, measurement := range input.Measurements {
		symbols[measurement.Label] = struct{}{}
	}

	for symbol := range symbols {
		rehearsal.admitObservation(reference, symbol, capture.ReceivedAt)
	}
	return nil
}

// admitObservation locates a decision input on its own symbol's causal tape.
// An envelope may contain several symbols, as it does on the live learner.
func (rehearsal *Rehearsal) admitObservation(reference hindsight.EnvelopeRef, symbol string, at time.Time) {
	observations := rehearsal.observations[symbol]
	index := sort.Search(len(observations), func(index int) bool {
		return observations[index].Capture.Sequence > reference.Origin.Sequence ||
			(observations[index].Capture.Sequence == reference.Origin.Sequence && observations[index].Ordinal >= reference.Ordinal)
	})
	if index < len(observations) && observations[index].Capture == reference.Origin && observations[index].Ordinal == reference.Ordinal {
		return
	}
	observation := hindsight.Observation{Domain: "spot", Symbol: symbol}

	for previous := index - 1; previous >= 0; previous-- {
		if observations[previous].Kind == "ticker" || observations[previous].Kind == "l3_touch" {
			observation = observations[previous]
			break
		}
	}
	observation.Capture, observation.Ordinal = reference.Origin, reference.Ordinal
	observation.Kind, observation.ReceivedAt, observation.VenueAt = "precursor", at, at
	observations = append(observations, hindsight.Observation{})
	copy(observations[index+1:], observations[index:])
	observations[index] = observation
	rehearsal.observations[symbol] = observations
	return
}
