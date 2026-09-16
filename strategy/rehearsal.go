package strategy

import (
	"cmp"
	"context"
	"iter"
	"slices"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/strategy/impulse"
)

const TrainingFormat = "symm-volume-training/1"

type trainingReading struct {
	Learned, Resolved, Correct, Predicted, Entered, Profitable, Unsupported uint64
	Return                                                                  float64
}

type trainingAnchor struct {
	sequence   int64
	key        []byte
	prediction string
}

/*
Rehearsal owns delayed supervision. The storage drain delivers complete,
immutable producer boundaries in order. A separate map reconstructs their
regions without retaining market history. Only one open anchor per symbol is
retained. Predictions are frozen before future outcomes arrive. The live path
reads the shared trie and an atomic progress publication; it never waits here.
*/
type Rehearsal struct {
	ctx       context.Context
	engine    *cognition.Engine
	detector  *tables.StreamingDetector
	space     *impulse.Map
	precursor *Precursor
	anchors   map[string]trainingAnchor
	reading   trainingReading
	published atomic.Pointer[trainingReading]
	sequence  int64
	records   []tables.ExcursionRecord
}

func NewRehearsal(ctx context.Context, epoch int64, price *broker.Price, engine *cognition.Engine) *Rehearsal {
	return &Rehearsal{ctx: ctx, engine: engine, detector: tables.NewStreamingDetector(epoch, price),
		space: impulse.NewMap(), precursor: NewPrecursor(), anchors: make(map[string]trainingAnchor)}
}

// Step consumes the existing training publication, whose peers are its sealed inputs.
func (rehearsal *Rehearsal) Step(frame *data.Measurement[float64]) ([]tables.ExcursionRecord, error) {
	if frame.Metrics["previous_input"].Raw != float64(rehearsal.sequence) ||
		frame.Metrics["input_count"].Raw != float64(len(frame.Peers)) ||
		frame.Metrics["impulse_version"].Raw != grid.FormatVersion {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: incomplete producer boundary", nil))
	}

	if err := rehearsal.space.Step(frame); err != nil {
		return nil, err
	}

	rehearsal.records = rehearsal.records[:0]

	for _, measurement := range frame.Peers {
		record, err := rehearsal.detector.Process(measurement)

		if err != nil {
			return nil, err
		}

		if record != nil {
			if err := rehearsal.resolve(*record, &rehearsal.space.Markets[record.Symbol].Impulse); err != nil {
				return nil, err
			}
			rehearsal.records = append(rehearsal.records, *record)
		}

		if rehearsal.detector.Anchor(measurement.Label) == frame.SeqIdx && rehearsal.anchors[measurement.Label].sequence != frame.SeqIdx {
			market := rehearsal.space.Markets[measurement.Label]
			if err := rehearsal.capture(&market.Impulse); err != nil {
				return nil, err
			}
		}
	}

	rehearsal.sequence = frame.SeqIdx
	if len(rehearsal.records) > 0 {
		reading := rehearsal.reading
		rehearsal.published.Store(&reading)
	}
	return rehearsal.records, nil
}

func (rehearsal *Rehearsal) capture(reading *grid.Impulse) error {
	key := rehearsal.precursor.Encode(reading, false)
	anchor := trainingAnchor{sequence: reading.SeqIdx, key: slices.Clone(key)}
	command := cognition.Command{Evaluate: &cognition.Question{Context: key, Exact: true}}
	pipeline := nomagique.NewNumber(rehearsal.engine)
	input := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(&command)) }

	for output := range pipeline.Next(input) {
		anchor.prediction = (*cognition.Evaluation)(output).WinnerClass
	}

	if err := pipeline.Error(); err != nil {
		return errnie.Error(err)
	}

	rehearsal.anchors[reading.Label] = anchor
	return nil
}

func (rehearsal *Rehearsal) resolve(record tables.ExcursionRecord, exit *grid.Impulse) error {
	if exit == nil || exit.Label != record.Symbol || exit.SeqIdx != record.ExitTick {
		return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: outcome exit market absent", nil))
	}

	anchor, found := rehearsal.anchors[record.Symbol]

	if !found || anchor.sequence != record.AnchorTick {
		return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: outcome has no matching anchor", nil))
	}

	if len(anchor.key) == 0 {
		rehearsal.reading.Unsupported++
		return nil
	}

	action := ActionWait

	if record.ClearsFriction {
		action = ActionEnter
	}

	// Score the held-out prediction before this outcome updates the trie.
	rehearsal.reading.Resolved++

	if anchor.prediction != "" {
		rehearsal.reading.Predicted++
		if anchor.prediction == string(action) {
			rehearsal.reading.Correct++
		}
	}

	if anchor.prediction == string(ActionEnter) {
		rehearsal.reading.Entered++
		rehearsal.reading.Return += record.ProfitFraction
		if record.ProfitFraction > 0 {
			rehearsal.reading.Profitable++
		}
	}

	association := cognition.Association{Context: anchor.key, Class: []byte(action)}
	if _, err := rehearsal.engine.Observe(association); err != nil {
		return errnie.Error(err)
	}

	rehearsal.reading.Learned++
	// At a detected regime boundary the completed virtual position is closed.
	// The holding bit keeps this lifecycle label separate from entry selection.
	key := rehearsal.precursor.Encode(exit, true)
	if len(key) > 0 {
		if _, err := rehearsal.engine.Observe(cognition.Association{Context: key, Class: []byte(ActionExit)}); err != nil {
			return errnie.Error(err)
		}
		rehearsal.reading.Learned++
	}
	return nil
}

/*
	Replay reconstructs archived contexts before applying each delayed label.

Incomplete tapes and missing anchors fail; outcomes never enter map inputs.
*/
func (rehearsal *Rehearsal) Replay(records []tables.ExcursionRecord, frames iter.Seq2[*data.Measurement[float64], error]) error {
	space := impulse.NewMap()
	anchors := make(map[int64][]tables.ExcursionRecord)
	exits := make(map[int64][]tables.ExcursionRecord)

	seen := make(map[string]bool)
	for _, record := range records {
		if record.ID == "" || seen[record.ID] {
			return errnie.Error(errnie.Err(errnie.Conflict, "rehearsal: missing or duplicate outcome identity", nil))
		}
		seen[record.ID] = true
		if record.AnchorTick <= 0 || record.ExitTick <= record.AnchorTick {
			return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: invalid outcome boundaries", nil))
		}
		anchors[record.AnchorTick] = append(anchors[record.AnchorTick], record)
		exits[record.ExitTick] = append(exits[record.ExitTick], record)
	}

	for frame, err := range frames {
		if err != nil {
			return errnie.Error(err)
		}
		if err := rehearsal.ctx.Err(); err != nil {
			return errnie.Error(err)
		}
		if err := space.Step(frame); err != nil {
			return err
		}

		for _, record := range exits[frame.SeqIdx] {
			market := space.Markets[record.Symbol]
			if market == nil {
				return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: exit market absent", nil))
			}
			if err := rehearsal.resolve(record, &market.Impulse); err != nil {
				return err
			}
		}
		delete(exits, frame.SeqIdx)

		for _, record := range anchors[frame.SeqIdx] {
			market := space.Markets[record.Symbol]
			if market == nil || market.Sequence != frame.SeqIdx {
				return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: anchor market absent", nil))
			}
			if err := rehearsal.capture(&market.Impulse); err != nil {
				return err
			}
		}
		delete(anchors, frame.SeqIdx)
	}

	if len(anchors) != 0 || len(exits) != 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: tape ended before outcome boundaries", nil))
	}

	clear(rehearsal.anchors)
	return nil
}

// Restore replays completed volume-training runs before root opens market ingress.
func (rehearsal *Rehearsal) Restore(catalog *tables.Catalog) error {
	runs, err := catalog.Runs(rehearsal.ctx)
	if err != nil {
		return errnie.Error(err)
	}
	slices.SortFunc(runs, func(left, right tables.Run) int { return cmp.Compare(left.Epoch, right.Epoch) })
	for _, run := range runs {
		if run.BuildID != TrainingFormat {
			continue
		}
		records, err := catalog.Excursions(rehearsal.ctx, run.Epoch, nil)
		if err != nil {
			return errnie.Error(err)
		}
		if len(records) == 0 {
			continue
		}
		var through int64
		for _, record := range records {
			through = max(through, record.ExitTick)
		}
		records, frames, err := catalog.Replay(rehearsal.ctx, run.Epoch, through)
		if err != nil {
			return errnie.Error(err)
		}
		if err := rehearsal.Replay(records, frames); err != nil {
			return err
		}
	}
	reading := rehearsal.reading
	rehearsal.published.Store(&reading)
	return nil
}
