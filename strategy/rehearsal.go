package strategy

import (
	"bytes"
	"cmp"
	"context"
	"encoding/gob"
	"fmt"
	"iter"
	"os"
	"path/filepath"
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

const TrainingFormat = "symm-volume-training/2"

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
type anchorKey struct {
	symbol   string
	sequence int64
}

type Rehearsal struct {
	ctx             context.Context
	engine          *cognition.Engine
	detector        *tables.StreamingDetector
	space           *impulse.Map
	precursor       *Precursor
	anchors         map[anchorKey]trainingAnchor
	reading         trainingReading
	published       atomic.Pointer[trainingReading]
	sequence        int64
	records         []tables.ExcursionRecord
	checkpointEpoch int64
}

func NewRehearsal(ctx context.Context, epoch int64, price *broker.Price, engine *cognition.Engine) *Rehearsal {
	return &Rehearsal{
		ctx:       ctx,
		engine:    engine,
		detector:  tables.NewStreamingDetector(epoch, price),
		space:     impulse.NewMap(),
		precursor: NewPrecursor(),
		anchors:   make(map[anchorKey]trainingAnchor),
	}
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

		if rehearsal.detector.Anchor(measurement.Label) == frame.SeqIdx {
			key := anchorKey{symbol: measurement.Label, sequence: frame.SeqIdx}
			market := rehearsal.space.Markets[measurement.Label]

			if _, exists := rehearsal.anchors[key]; !exists && market != nil {
				if err := rehearsal.capture(&market.Impulse); err != nil {
					return nil, err
				}
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
	command := cognition.Command{Evaluate: &cognition.Question{Context: key, Exact: false}}
	pipeline := nomagique.NewNumber(rehearsal.engine)
	input := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(&command)) }

	for output := range pipeline.Next(input) {
		anchor.prediction = (*cognition.Evaluation)(output).WinnerClass
	}

	if err := pipeline.Error(); err != nil {
		return errnie.Error(err)
	}

	rehearsal.anchors[anchorKey{symbol: reading.Label, sequence: reading.SeqIdx}] = anchor
	return nil
}

func (rehearsal *Rehearsal) train(sequence []byte, class []byte, feedback float64) (cognition.Result, error) {
	if len(sequence) == 0 || len(class) == 0 {
		return cognition.Result{}, nil
	}

	res, err := rehearsal.engine.Observe(cognition.Association{
		Context:  sequence,
		Class:    class,
		Feedback: feedback,
		Graded:   true,
	})

	if err != nil {
		return res, err
	}

	if len(sequence)%8 == 0 && len(sequence) > 16 {
		tokens := len(sequence) / 8
		maxOrder := 8
		limit := min(tokens-1, maxOrder)

		for endIdx := 2; endIdx <= limit; endIdx++ {
			subContext := sequence[:endIdx*8]

			if _, err := rehearsal.engine.Observe(cognition.Association{
				Context:  subContext,
				Class:    class,
				Feedback: feedback,
				Graded:   true,
			}); err != nil {
				return res, err
			}
		}
	}

	return res, nil
}

func (rehearsal *Rehearsal) resolve(record tables.ExcursionRecord, exit *grid.Impulse) error {
	if exit == nil || exit.Label != record.Symbol || exit.SeqIdx != record.ExitTick {
		return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: outcome exit market absent", nil))
	}

	key := anchorKey{symbol: record.Symbol, sequence: record.AnchorTick}
	anchor, found := rehearsal.anchors[key]

	if !found {
		rehearsal.reading.Unsupported++
		return nil
	}

	delete(rehearsal.anchors, key)

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

	if _, err := rehearsal.train(anchor.key, []byte(ActionEnter), record.ProfitFraction); err != nil {
		return errnie.Error(err)
	}

	rehearsal.reading.Learned++

	if _, err := rehearsal.train(anchor.key, []byte(ActionWait), -record.ProfitFraction); err != nil {
		return errnie.Error(err)
	}

	rehearsal.reading.Learned++
	// At a detected regime boundary the completed virtual position is closed.
	// The holding bit keeps this lifecycle label separate from entry selection.
	exitKey := rehearsal.precursor.Encode(exit, true)

	if len(exitKey) > 0 {
		if _, err := rehearsal.train(exitKey, []byte(ActionExit), 1.0); err != nil {
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

type RehearsalCheckpoint struct {
	Format  string
	Epoch   int64
	Reading trainingReading
	Model   []byte
}

func (rehearsal *Rehearsal) SaveCheckpoint(epoch int64) error {
	if rehearsal == nil || rehearsal.engine == nil {
		return nil
	}

	result, err := rehearsal.engine.Snapshot()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "rehearsal: snapshot failed", err))
	}

	checkpoint := RehearsalCheckpoint{
		Format:  TrainingFormat,
		Epoch:   epoch,
		Reading: rehearsal.reading,
		Model:   result.Model,
	}

	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	if err := encoder.Encode(checkpoint); err != nil {
		return errnie.Error(errnie.Err(errnie.UnprocessableContent, "rehearsal: checkpoint encode failed", err))
	}

	if err := os.MkdirAll("runs", 0o755); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "rehearsal: mkdir runs failed", err))
	}

	targetPath := filepath.Join("runs", "rehearsal_checkpoint.bin")
	temporaryPath := targetPath + ".tmp"

	if err := os.WriteFile(temporaryPath, buffer.Bytes(), 0o600); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "rehearsal: write checkpoint failed", err))
	}

	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "rehearsal: rename checkpoint failed", err))
	}

	errnie.Info(fmt.Sprintf(
		"rehearsal: saved checkpoint for epoch %d (%d bytes, %d situations learned)",
		epoch, buffer.Len(), checkpoint.Reading.Learned,
	))

	rehearsal.checkpointEpoch = epoch
	return nil
}

func (rehearsal *Rehearsal) LoadCheckpoint() (int64, error) {
	if rehearsal == nil || rehearsal.engine == nil {
		return 0, nil
	}

	if rehearsal.checkpointEpoch > 0 {
		return rehearsal.checkpointEpoch, nil
	}

	targetPath := filepath.Join("runs", "rehearsal_checkpoint.bin")
	payload, err := os.ReadFile(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, errnie.Error(errnie.Err(errnie.IO, "rehearsal: read checkpoint failed", err))
	}

	var checkpoint RehearsalCheckpoint
	decoder := gob.NewDecoder(bytes.NewReader(payload))

	if err := decoder.Decode(&checkpoint); err != nil {
		return 0, errnie.Error(errnie.Err(errnie.UnprocessableContent, "rehearsal: decode checkpoint failed", err))
	}

	if checkpoint.Format != TrainingFormat || len(checkpoint.Model) == 0 {
		return 0, nil
	}

	if _, err := rehearsal.engine.Restore(checkpoint.Model); err != nil {
		return 0, errnie.Error(errnie.Err(errnie.Internal, "rehearsal: restore model failed", err))
	}

	rehearsal.checkpointEpoch = checkpoint.Epoch
	rehearsal.reading = checkpoint.Reading
	rehearsal.published.Store(&rehearsal.reading)

	errnie.Info(fmt.Sprintf(
		"rehearsal: restored checkpoint for epoch %d (learned=%d, return=%.4f)",
		checkpoint.Epoch, checkpoint.Reading.Learned, checkpoint.Reading.Return,
	))

	return checkpoint.Epoch, nil
}

// Restore replays completed volume-training runs before root opens market ingress.
func (rehearsal *Rehearsal) Restore(catalog *tables.Catalog) error {
	lastEpoch, err := rehearsal.LoadCheckpoint()

	if err != nil {
		errnie.Error(err)
	}

	errnie.Info("rehearsal: loading runs from catalog...")
	runs, err := catalog.Runs(rehearsal.ctx)

	if err != nil {
		return errnie.Error(err)
	}

	errnie.Info(fmt.Sprintf("rehearsal: loaded %d runs from catalog", len(runs)))
	slices.SortFunc(runs, func(left, right tables.Run) int {
		return cmp.Compare(left.Epoch, right.Epoch)
	})

	var pending []tables.Run

	for _, run := range runs {
		if run.BuildID == TrainingFormat && run.Epoch > lastEpoch {
			pending = append(pending, run)
		}
	}

	if len(pending) == 0 {
		errnie.Info("rehearsal: model is up-to-date with checkpoint; 0 runs to replay")
		return nil
	}

	var latestEpoch int64

	for _, run := range pending {
		records, err := catalog.Excursions(rehearsal.ctx, run.Epoch, nil)

		if err != nil {
			return errnie.Error(err)
		}

		errnie.Info(fmt.Sprintf("rehearsal: run epoch=%d has %d excursion records", run.Epoch, len(records)))

		if len(records) == 0 {
			continue
		}

		var through int64

		for _, record := range records {
			through = max(through, record.ExitTick)
		}

		errnie.Info(fmt.Sprintf("rehearsal: replaying run epoch=%d through tick=%d...", run.Epoch, through))
		records, frames, err := catalog.Replay(rehearsal.ctx, run.Epoch, through)

		if err != nil {
			return errnie.Error(err)
		}

		if err := rehearsal.Replay(records, frames); err != nil {
			return err
		}

		latestEpoch = run.Epoch
		errnie.Info(fmt.Sprintf("rehearsal: finished replaying run epoch=%d", run.Epoch))
	}

	reading := rehearsal.reading
	rehearsal.published.Store(&reading)

	if latestEpoch > 0 {
		if err := rehearsal.SaveCheckpoint(latestEpoch); err != nil {
			errnie.Error(err)
		}
	}

	errnie.Info("rehearsal: restore complete")
	return nil
}
