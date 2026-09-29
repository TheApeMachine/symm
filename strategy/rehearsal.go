package strategy

import (
	"bytes"
	"cmp"
	"context"
	"encoding/gob"
	"fmt"
	"iter"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	Learned                 uint64
	Resolved                uint64
	Correct                 uint64
	Predicted               uint64
	Entered                 uint64
	Profitable              uint64
	Losing                  uint64
	Unsupported             uint64
	FragmentsUp             uint64
	FragmentsDown           uint64
	FragmentsChop           uint64
	FragmentsFlat           uint64
	FragmentsUnsupported    uint64
	ValidUpOpportunities    uint64
	CorrectEnter            uint64
	MissedEnter             uint64
	FalseEnterFriction      uint64
	FalseEnterDown          uint64
	FalseEnterChop          uint64
	FalseEnterFlat          uint64
	CorrectWaitDown         uint64
	CorrectWaitChop         uint64
	CorrectWaitFlat         uint64
	CorrectWaitFriction     uint64
	ContinuationWaitCorrect uint64
	PrematureExit           uint64
	CorrectExit             uint64
	MissedExit              uint64
	Return                  float64
	ReturnSq                float64
	MeanReturn              float64
	ReturnSE                float64
	LowerBound              float64
}

func (reading *trainingReading) countFragment(direction string) {
	switch strings.ToUpper(direction) {
	case "UP":
		reading.FragmentsUp++
	case "DOWN":
		reading.FragmentsDown++
	case "CHOP":
		reading.FragmentsChop++
	case "FLAT":
		reading.FragmentsFlat++
	}
}

func (reading *trainingReading) updateEconomics(profitFraction float64) {
	reading.Entered++
	reading.Return += profitFraction
	reading.ReturnSq += profitFraction * profitFraction

	if profitFraction > 0 {
		reading.Profitable++
	}

	if profitFraction < 0 {
		reading.Losing++
	}

	if reading.Entered >= 2 {
		n := float64(reading.Entered)
		mean := reading.Return / n
		variance := (reading.ReturnSq - (reading.Return*reading.Return)/n) / (n - 1.0)

		if variance < 0 {
			variance = 0
		}

		se := math.Sqrt(variance / n)
		reading.MeanReturn = mean
		reading.ReturnSE = se
		reading.LowerBound = mean - se
	}
}

type trainingAnchor struct {
	sequence   int64
	key        []byte
	tokens     []uint64
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

type ReplayProgressFunc func(frame *data.Measurement[float64], record *tables.ExcursionRecord, impulse *grid.Snapshot, prediction string, tokens []uint64)

type Rehearsal struct {
	ctx              context.Context
	engine           *cognition.Engine
	price            *broker.Price
	detector         *tables.StreamingDetector
	space            *impulse.Map
	precursor        *Precursor
	anchors          map[anchorKey]trainingAnchor
	reading          trainingReading
	readingMu        sync.RWMutex
	published        atomic.Pointer[trainingReading]
	lastExcursion    atomic.Pointer[tables.ExcursionRecord]
	lastExcursions   map[string]*tables.ExcursionRecord
	lastExcursionsMu sync.RWMutex
	sequence         int64
	records          []tables.ExcursionRecord
	checkpointEpoch  atomic.Int64
	trainedRuns      map[int64]int64
	trainedRunsMu    sync.RWMutex
	onProgress       ReplayProgressFunc
	isSaving         atomic.Bool
	isReplaying      atomic.Bool
}

func (rehearsal *Rehearsal) LastExcursion(symbol ...string) *tables.ExcursionRecord {
	if rehearsal == nil {
		return nil
	}

	if len(symbol) > 0 && symbol[0] != "" {
		rehearsal.lastExcursionsMu.RLock()
		defer rehearsal.lastExcursionsMu.RUnlock()

		if rehearsal.lastExcursions != nil {
			if rec, exists := rehearsal.lastExcursions[symbol[0]]; exists {
				return rec
			}
		}

		return nil
	}

	return rehearsal.lastExcursion.Load()
}

func (rehearsal *Rehearsal) TrainedRunsSnapshot() map[int64]int64 {
	if rehearsal == nil {
		return make(map[int64]int64)
	}

	rehearsal.trainedRunsMu.RLock()
	defer rehearsal.trainedRunsMu.RUnlock()

	snapshot := make(map[int64]int64, len(rehearsal.trainedRuns))

	for epoch, tick := range rehearsal.trainedRuns {
		snapshot[epoch] = tick
	}

	return snapshot
}

func (rehearsal *Rehearsal) RecordTrainedRun(epoch int64, tick int64) {
	if rehearsal == nil {
		return
	}

	rehearsal.trainedRunsMu.Lock()
	defer rehearsal.trainedRunsMu.Unlock()

	if rehearsal.trainedRuns == nil {
		rehearsal.trainedRuns = make(map[int64]int64)
	}

	rehearsal.trainedRuns[epoch] = tick
}

func (rehearsal *Rehearsal) MergeTrainedRuns(runs map[int64]int64) {
	if rehearsal == nil || len(runs) == 0 {
		return
	}

	rehearsal.trainedRunsMu.Lock()
	defer rehearsal.trainedRunsMu.Unlock()

	if rehearsal.trainedRuns == nil {
		rehearsal.trainedRuns = make(map[int64]int64, len(runs))
	}

	for epoch, tick := range runs {
		rehearsal.trainedRuns[epoch] = tick
	}
}

func (rehearsal *Rehearsal) SetOnProgress(callback ReplayProgressFunc) {
	rehearsal.onProgress = callback
}

func NewRehearsal(ctx context.Context, epoch int64, price *broker.Price, engine *cognition.Engine) *Rehearsal {
	return &Rehearsal{
		ctx:            ctx,
		engine:         engine,
		price:          price,
		detector:       tables.NewStreamingDetector(epoch, price),
		space:          impulse.NewMap(),
		precursor:      NewPrecursor(),
		anchors:        make(map[anchorKey]trainingAnchor),
		trainedRuns:    make(map[int64]int64),
		lastExcursions: make(map[string]*tables.ExcursionRecord),
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

	for _, market := range rehearsal.space.Markets {
		rehearsal.precursor.Step(&market.Impulse)
	}

	for _, measurement := range frame.Peers {
		record, err := rehearsal.detector.Process(measurement)

		if err != nil {
			return nil, err
		}

		if record != nil {
			if _, _, err := rehearsal.resolve(*record, &rehearsal.space.Markets[record.Symbol].Impulse); err != nil {
				return nil, err
			}

			rehearsal.records = append(rehearsal.records, *record)
			copied := *record
			rehearsal.lastExcursion.Store(&copied)

			rehearsal.lastExcursionsMu.Lock()
			if rehearsal.lastExcursions == nil {
				rehearsal.lastExcursions = make(map[string]*tables.ExcursionRecord)
			}
			rehearsal.lastExcursions[record.Symbol] = &copied
			rehearsal.lastExcursionsMu.Unlock()
		}

		if rehearsal.detector.Anchor(measurement.Label) == frame.SeqIdx {
			key := anchorKey{symbol: measurement.Label, sequence: frame.SeqIdx}
			market := rehearsal.space.Markets[measurement.Label]

			if _, exists := rehearsal.anchors[key]; !exists && market != nil {
				if err := rehearsal.capture(&market.Impulse); err != nil {
					return nil, err
				}
				rehearsal.precursor.Reset(measurement.Label)
			}
		}
	}

	rehearsal.sequence = frame.SeqIdx

	if epoch := rehearsal.checkpointEpoch.Load(); epoch > 0 {
		rehearsal.RecordTrainedRun(epoch, frame.SeqIdx)
	}

	if len(rehearsal.records) > 0 {
		rehearsal.readingMu.RLock()
		reading := rehearsal.reading
		rehearsal.readingMu.RUnlock()
		rehearsal.published.Store(&reading)
	}

	return rehearsal.records, nil
}

func (rehearsal *Rehearsal) evaluateContext(contextKey []byte) string {
	if len(contextKey) == 0 {
		return ""
	}

	command := cognition.Command{Evaluate: &cognition.Question{Context: contextKey, Exact: false}}
	pipeline := nomagique.NewNumber(rehearsal.engine)
	input := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(&command)) }

	winner := ""

	for output := range pipeline.Next(input) {
		winner = (*cognition.Evaluation)(output).WinnerClass
	}

	return winner
}

func (rehearsal *Rehearsal) capture(reading *grid.Impulse) error {
	rehearsal.precursor.Step(reading)
	tokens := rehearsal.precursor.Tokens(reading.Label)

	if len(tokens) == 0 {
		tok := ImpulseToken(reading.Regions)
		if tok > 0 {
			tokens = []uint64{tok}
		}
	}

	key := EncodeTokens(reading.Label, false, tokens)

	anchor := trainingAnchor{
		sequence:   reading.SeqIdx,
		key:        slices.Clone(key),
		tokens:     slices.Clone(tokens),
		prediction: rehearsal.evaluateContext(key),
	}

	rehearsal.anchors[anchorKey{symbol: reading.Label, sequence: reading.SeqIdx}] = anchor
	return nil
}

func (rehearsal *Rehearsal) train(sequence []byte, class []byte) (cognition.Result, error) {
	if len(sequence) == 0 || len(class) == 0 {
		return cognition.Result{}, nil
	}

	return rehearsal.engine.Observe(cognition.Association{
		Context: sequence,
		Class:   class,
		Graded:  false,
	})
}

func (rehearsal *Rehearsal) resolve(record tables.ExcursionRecord, exit *grid.Impulse) (string, []uint64, error) {
	if exit == nil || exit.Label != record.Symbol || exit.SeqIdx != record.ExitTick {
		return "", nil, errnie.Error(errnie.Err(errnie.Validation, "rehearsal: outcome exit market absent", nil))
	}

	if record.Direction == "" {
		record.Direction = "DOWN"
		if record.ProfitFraction > 0 {
			record.Direction = "UP"
		}
	}
	record.Direction = strings.ToUpper(record.Direction)

	key := anchorKey{symbol: record.Symbol, sequence: record.AnchorTick}
	anchor, found := rehearsal.anchors[key]
	delete(rehearsal.anchors, key)

	rehearsal.readingMu.Lock()
	defer rehearsal.readingMu.Unlock()

	rehearsal.reading.countFragment(record.Direction)

	if !found || len(anchor.key) == 0 || record.Status == "unsupported" {
		rehearsal.reading.Unsupported++
		rehearsal.reading.FragmentsUnsupported++
		return "", nil, nil
	}

	rehearsal.reading.Resolved++

	isUsefulUp := record.Direction == "UP" && record.ClearsFriction && record.ProfitFraction > 0
	expectedAction := ActionWait

	if isUsefulUp {
		expectedAction = ActionEnter
		rehearsal.reading.ValidUpOpportunities++
	}

	if anchor.prediction != "" {
		rehearsal.reading.Predicted++

		if anchor.prediction == string(expectedAction) {
			rehearsal.reading.Correct++
		}

		if isUsefulUp {
			if anchor.prediction == string(ActionEnter) {
				rehearsal.reading.CorrectEnter++
			}

			if anchor.prediction != string(ActionEnter) {
				rehearsal.reading.MissedEnter++
			}
		}

		if !isUsefulUp {
			if anchor.prediction == string(ActionEnter) {
				switch record.Direction {
				case "UP":
					rehearsal.reading.FalseEnterFriction++
				case "DOWN":
					rehearsal.reading.FalseEnterDown++
				case "CHOP":
					rehearsal.reading.FalseEnterChop++
				case "FLAT":
					rehearsal.reading.FalseEnterFlat++
				}
			}

			if anchor.prediction == string(ActionWait) {
				switch record.Direction {
				case "UP":
					rehearsal.reading.CorrectWaitFriction++
				case "DOWN":
					rehearsal.reading.CorrectWaitDown++
				case "CHOP":
					rehearsal.reading.CorrectWaitChop++
				case "FLAT":
					rehearsal.reading.CorrectWaitFlat++
				}
			}
		}

		if anchor.prediction == string(ActionEnter) {
			rehearsal.reading.updateEconomics(record.ProfitFraction)
		}
	}

	excursionTokens := rehearsal.precursor.Tokens(record.Symbol)
	holdingTokens := append(slices.Clone(anchor.tokens), excursionTokens...)

	if isUsefulUp && len(excursionTokens) > 0 {
		for j := 0; j < len(excursionTokens)-1; j++ {
			holdingPrefix := append(slices.Clone(anchor.tokens), excursionTokens[:j+1]...)
			holdingPrefixKey := EncodeTokens(record.Symbol, true, holdingPrefix)
			pred := rehearsal.evaluateContext(holdingPrefixKey)

			if pred == string(ActionExit) {
				rehearsal.reading.PrematureExit++
			}

			if pred == string(ActionWait) {
				rehearsal.reading.ContinuationWaitCorrect++
			}
		}

		holdingExitKey := EncodeTokens(record.Symbol, true, holdingTokens)
		exitPred := rehearsal.evaluateContext(holdingExitKey)

		if exitPred == string(ActionExit) {
			rehearsal.reading.CorrectExit++
		}

		if exitPred != string(ActionExit) {
			rehearsal.reading.MissedExit++
		}
	}

	// 1. Prefixes before B teach ActionWait
	for j := 1; j < len(anchor.tokens); j++ {
		prefixKey := EncodeTokens(record.Symbol, false, anchor.tokens[:j])

		if _, err := rehearsal.train(prefixKey, []byte(ActionWait)); err != nil {
			return "", nil, errnie.Error(err)
		}

		rehearsal.reading.Learned++
	}

	// 2. Complete context at B teaches expectedAction (ENTER if useful UP, else WAIT)
	if _, err := rehearsal.train(anchor.key, []byte(expectedAction)); err != nil {
		return "", nil, errnie.Error(err)
	}

	rehearsal.reading.Learned++

	// 3. Holding-scope exit supervision
	if isUsefulUp && len(excursionTokens) > 0 {
		for j := 0; j < len(excursionTokens)-1; j++ {
			holdingPrefix := append(slices.Clone(anchor.tokens), excursionTokens[:j+1]...)
			holdingPrefixKey := EncodeTokens(record.Symbol, true, holdingPrefix)

			if _, err := rehearsal.train(holdingPrefixKey, []byte(ActionWait)); err != nil {
				return "", nil, errnie.Error(err)
			}

			rehearsal.reading.Learned++
		}

		holdingExitKey := EncodeTokens(record.Symbol, true, holdingTokens)

		if _, err := rehearsal.train(holdingExitKey, []byte(ActionExit)); err != nil {
			return "", nil, errnie.Error(err)
		}

		rehearsal.reading.Learned++
	}

	return anchor.prediction, anchor.tokens, nil
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

	var stepCount int64

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
		stepCount++

		for _, market := range space.Markets {
			rehearsal.precursor.Step(&market.Impulse)
		}

		resolvedRecord := false

		for _, record := range exits[frame.SeqIdx] {
			market := space.Markets[record.Symbol]
			if market == nil {
				return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: exit market absent", nil))
			}
			pred, anchorTokens, err := rehearsal.resolve(record, &market.Impulse)
			if err != nil {
				return err
			}

			resolvedRecord = true

			if rehearsal.onProgress != nil {
				rehearsal.onProgress(frame, &record, market.Snapshot(), pred, anchorTokens)
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
			rehearsal.precursor.Reset(record.Symbol)
		}
		delete(anchors, frame.SeqIdx)

		if !resolvedRecord && rehearsal.onProgress != nil && stepCount%50 == 0 {
			market := space.Markets[frame.Label]
			var snapshot *grid.Snapshot

			if market != nil {
				snapshot = market.Snapshot()
			}

			rehearsal.onProgress(frame, nil, snapshot, "", nil)
		}
	}

	if len(anchors) != 0 || len(exits) != 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "rehearsal: tape ended before outcome boundaries", nil))
	}

	clear(rehearsal.anchors)
	return nil
}

type RehearsalCheckpoint struct {
	Format      string
	Epoch       int64
	TrainedRuns map[int64]int64
	Reading     trainingReading
	Model       []byte
}

func (rehearsal *Rehearsal) SaveCheckpoint(epoch int64) error {
	if rehearsal == nil || rehearsal.engine == nil {
		return nil
	}

	if !rehearsal.isSaving.CompareAndSwap(false, true) {
		return nil
	}
	defer rehearsal.isSaving.Store(false)

	result, err := rehearsal.engine.Snapshot()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "rehearsal: snapshot failed", err))
	}

	rehearsal.readingMu.RLock()
	reading := rehearsal.reading
	rehearsal.readingMu.RUnlock()

	if published := rehearsal.published.Load(); published != nil {
		reading = *published
	}

	if epoch > 0 {
		rehearsal.RecordTrainedRun(epoch, math.MaxInt64)
	}

	checkpoint := RehearsalCheckpoint{
		Format:      TrainingFormat,
		Epoch:       epoch,
		TrainedRuns: rehearsal.TrainedRunsSnapshot(),
		Reading:     reading,
		Model:       result.Model,
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

	rehearsal.checkpointEpoch.Store(epoch)
	return nil
}

func (rehearsal *Rehearsal) LoadCheckpoint() (int64, error) {
	if rehearsal == nil || rehearsal.engine == nil {
		return 0, nil
	}

	if epoch := rehearsal.checkpointEpoch.Load(); epoch > 0 {
		return epoch, nil
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

	if checkpoint.TrainedRuns != nil {
		rehearsal.MergeTrainedRuns(checkpoint.TrainedRuns)
	}

	if checkpoint.Epoch > 0 {
		rehearsal.RecordTrainedRun(checkpoint.Epoch, math.MaxInt64)
	}

	rehearsal.checkpointEpoch.Store(checkpoint.Epoch)
	rehearsal.readingMu.Lock()
	rehearsal.reading = checkpoint.Reading
	rehearsal.readingMu.Unlock()
	rehearsal.published.Store(&checkpoint.Reading)

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

	if lastEpoch > 0 {
		rehearsal.RecordTrainedRun(lastEpoch, math.MaxInt64)
	}

	return rehearsal.ReplayPending(catalog)
}

// ReplayPending replays new excursions from the catalog without reloading or wiping the model.
func (rehearsal *Rehearsal) ReplayPending(catalog *tables.Catalog) error {
	if !rehearsal.isReplaying.CompareAndSwap(false, true) {
		errnie.Info("rehearsal: replay already in progress; skipping")
		return nil
	}
	defer rehearsal.isReplaying.Store(false)

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
	trainedSnapshot := rehearsal.TrainedRunsSnapshot()
	checkpointEpoch := rehearsal.checkpointEpoch.Load()

	for _, run := range runs {
		if run.BuildID != TrainingFormat && run.BuildID != "" {
			continue
		}

		if checkpointEpoch > 0 && run.Epoch <= checkpointEpoch {
			rehearsal.RecordTrainedRun(run.Epoch, math.MaxInt64)
			continue
		}

		records, err := catalog.Excursions(rehearsal.ctx, run.Epoch, nil)

		if err != nil || len(records) == 0 {
			continue
		}

		var through int64

		for _, record := range records {
			through = max(through, record.ExitTick)
		}

		trainedThrough, wasTrained := trainedSnapshot[run.Epoch]

		if wasTrained && through <= trainedThrough {
			continue
		}

		pending = append(pending, run)
	}

	if len(pending) == 0 {
		errnie.Info("rehearsal: model is up-to-date with checkpoint; 0 runs to replay")
		return nil
	}

	var latestEpoch int64

	for _, run := range pending {
		records, err := catalog.Excursions(rehearsal.ctx, run.Epoch, nil)

		if err != nil {
			errnie.Warn(fmt.Sprintf("rehearsal: skipping run epoch=%d excursions: %v", run.Epoch, err))
			continue
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
			errnie.Warn(fmt.Sprintf("rehearsal: skipping run epoch=%d replay: %v", run.Epoch, err))
			continue
		}

		if err := rehearsal.Replay(records, frames); err != nil {
			errnie.Warn(fmt.Sprintf("rehearsal: run epoch=%d replay failed: %v", run.Epoch, err))
			continue
		}

		rehearsal.RecordTrainedRun(run.Epoch, through)
		latestEpoch = run.Epoch
		errnie.Info(fmt.Sprintf("rehearsal: finished replaying run epoch=%d", run.Epoch))
	}

	rehearsal.readingMu.RLock()
	reading := rehearsal.reading
	rehearsal.readingMu.RUnlock()
	rehearsal.published.Store(&reading)

	if latestEpoch > 0 {
		if err := rehearsal.SaveCheckpoint(latestEpoch); err != nil {
			errnie.Error(err)
		}
	}

	errnie.Info("rehearsal: replay pending complete")
	return nil
}

// MergeReading cumulatively merges another trainingReading into this rehearsal instance.
func (rehearsal *Rehearsal) MergeReading(delta trainingReading) {
	if rehearsal == nil {
		return
	}

	rehearsal.readingMu.Lock()
	rehearsal.reading.Learned += delta.Learned
	rehearsal.reading.Resolved += delta.Resolved
	rehearsal.reading.Correct += delta.Correct
	rehearsal.reading.Predicted += delta.Predicted
	rehearsal.reading.Entered += delta.Entered
	rehearsal.reading.Profitable += delta.Profitable
	rehearsal.reading.Unsupported += delta.Unsupported
	rehearsal.reading.Return += delta.Return
	rehearsal.reading.ReturnSq += delta.ReturnSq
	combined := rehearsal.reading
	rehearsal.readingMu.Unlock()

	rehearsal.published.Store(&combined)
}

// PollUntrained checks the catalog for new excursions and pre-trains on them using an independent replay worker.
func (rehearsal *Rehearsal) PollUntrained(catalog *tables.Catalog) error {
	if rehearsal == nil || rehearsal.engine == nil || catalog == nil {
		return nil
	}

	historical := NewRehearsal(rehearsal.ctx, rehearsal.checkpointEpoch.Load(), rehearsal.price, rehearsal.engine)
	historical.checkpointEpoch.Store(rehearsal.checkpointEpoch.Load())
	historical.MergeTrainedRuns(rehearsal.TrainedRunsSnapshot())

	if rehearsal.onProgress != nil {
		historical.SetOnProgress(rehearsal.onProgress)
	}

	if err := historical.ReplayPending(catalog); err != nil {
		return err
	}

	historical.readingMu.RLock()
	deltaReading := historical.reading
	historical.readingMu.RUnlock()

	rehearsal.MergeReading(deltaReading)
	rehearsal.MergeTrainedRuns(historical.TrainedRunsSnapshot())

	return nil
}
