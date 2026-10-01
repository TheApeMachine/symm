package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/ui"
)

const (
	gridKey   = "model/grid.json"
	engineKey = "model/cognition.gob"
	skillKey  = "model/skill.json"

	// supervisionCheckpointVersion rejects cognition/skill created under older
	// supervision rules (ProfitFraction edge, ±1 paper feedback, richest-row replay).
	supervisionCheckpointVersion = 2

	// persistCommitTimeout bounds Iceberg excursion commits so writing cannot
	// latch forever when the catalog/object store stalls (UI: durability blocked: writing).
	persistCommitTimeout = 45 * time.Second
)

var _ ui.CognitionSource = (*Training)(nil)

/*
Training develops one grid, checkpoints it with the trie, then grades resolved
episodes. Paper entries wait for a positive skill lower bound.
*/
type Training struct {
	*runtime.System
	grid     *store.Grid
	engine   *cognition.Engine
	trader   *Trader
	catalog  *tables.Catalog
	price    *broker.Price
	tee      runtime.Tee
	epoch    int64
	detector *Detector

	mu         sync.Mutex
	cond       *sync.Cond
	frames     map[string][]*data.Measurement[float64]
	signatures map[string][]byte
	entries    map[string][]byte
	pending    []heldEpisode
	episodes   []heldEpisode
	queue      []heldEpisode
	graded     map[string]bool
	// augmented records "episodeID\x00aIndex" already taught as a recognition
	// variation so the infinite augment loop cannot inflate evidence Counts.
	augmented            map[string]bool
	skill                statistic.Moments
	paper                statistic.Moments
	paperTrades          float64
	paperPredictions     float64
	fragmentsUp          float64
	fragmentsDown        float64
	fragmentsChop        float64
	fragmentsFlat        float64
	fragmentsUnsupported float64
	histCorrectEnter     float64
	histMissedEnter      float64
	histFalseEnter       float64
	histCorrectExit      float64
	histMissedExit       float64
	histCorrectWait      float64
	histClears           float64
	histFeeFailUp        float64
	skillSamples         []float64
	returns              statistic.Moments
	returnSamples        []float64
	fragmentSpanSum      float64
	checkpointed         bool
	checkpointFailed     bool
	modelRevision        uint64
	snapshotRevision     uint64
	restored             bool
	replaying            bool
	blocked              bool
	writing              bool
	persistFailed        bool
	persistErr           error
	// deferredLearn holds live episodes persisted during historical replay;
	// cognition teaching waits until replay completes (replay isolation).
	deferredLearn []heldEpisode
	// deferredPaper holds reconciled paper grades arriving during replay.
	deferredPaper []paperGrade
	paperGraded          map[string]bool
	durable              chan struct{}
	durableOnce          sync.Once
	wake                 chan struct{}
	settleWake           chan struct{}
	episodeReady         chan struct{}
}

type paperGrade struct {
	symbol   string
	entryCtx []byte
	feedback float64
}

type heldEpisode struct {
	record tables.ExcursionRecord
	frames []*data.Measurement[float64]
}

type marker struct {
	action string
	entry  int64
	exit   int64
}

func NewTraining(
	ctx context.Context,
	epoch int64,
	price *broker.Price,
	trader *Trader,
	catalog *tables.Catalog,
	tee runtime.Tee,
) *Training {
	training := &Training{
		System:       runtime.NewSystem(ctx, "training", price),
		grid:         store.NewGrid(),
		engine:       cognition.NewEngine(cognition.Config{}),
		trader:       trader,
		catalog:      catalog,
		price:        price,
		tee:          tee,
		epoch:        epoch,
		detector:     NewDetector(price),
		frames:       make(map[string][]*data.Measurement[float64]),
		signatures:   make(map[string][]byte),
		entries:      make(map[string][]byte),
		graded:       make(map[string]bool),
		augmented:    make(map[string]bool),
		paperGraded:  make(map[string]bool),
		durable:      make(chan struct{}),
		wake:         make(chan struct{}, 1),
		settleWake:   make(chan struct{}, 1),
		episodeReady: make(chan struct{}, 1),
	}
	training.cond = sync.NewCond(&training.mu)
	training.Transition(runtime.INIT)

	if trader != nil {
		trader.OnPositionClosed(training.onPositionClosed)
	}

	go training.persistLoop()
	go training.checkpointLoop()

	return training
}

func (training *Training) Step(input *runtime.StageInput, output *data.Measurement[float64]) *data.Measurement[float64] {
	if input == nil {
		return nil
	}

	scratch := data.NewMeasurement[float64]("training:scratch", nil)
	scratch.Label = output.Label
	data.StampInterval(scratch, input.At(), input.From())

	// Merge all prior-stage output metrics onto the SCRATCH measurement so
	// training's internal methods (predict, detector) can read them from a single place.
	for _, prior := range input.AllPriorOutputs() {
		if prior == nil || prior.Metrics == nil {
			continue
		}

		for key, metric := range prior.Metrics {
			scratch.SetMetric(key, metric)
		}

		// Carry Result from cognition if available.
		if prior.Result != nil && scratch.Result == nil {
			scratch.Result = prior.Result
		}
	}

	// Also merge ingress metrics.
	if ingress := input.Ingress(); ingress != nil && ingress.Metrics != nil {
		for key, metric := range ingress.Metrics {
			if _, exists := scratch.LookupMetric(key); !exists {
				scratch.SetMetric(key, metric)
			}
		}
	}

	training.live(input, scratch, output)

	return output
}

func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	if training == nil || training.engine == nil {
		return cognition.CognitionTreeExport{}
	}

	return training.engine.TreeExport()
}

func (training *Training) Run() {
	go func() {
		select {
		case <-training.Context().Done():
			return
		case <-training.durable:
		}

		training.mu.Lock()
		training.replaying = true
		training.mu.Unlock()

		err := training.replayHistory()

		training.mu.Lock()
		training.replaying = false
		deferred := training.deferredLearn
		training.deferredLearn = nil
		deferredPaper := training.deferredPaper
		training.deferredPaper = nil
		training.mu.Unlock()

		if err != nil {
			training.Error(err)

			return
		}

		for _, episode := range deferred {
			training.supervise(episode, true)
		}
		for _, grade := range deferredPaper {
			training.applyPaperGrade(grade.symbol, grade.entryCtx, grade.feedback)
		}

		training.augment()
	}()
}

func (training *Training) live(input *runtime.StageInput, scratch, output *data.Measurement[float64]) {
	var gridInventory []*data.Measurement[float64]
	if input.Ingress() != nil {
		gridInventory = append(gridInventory, input.Ingress())
	}
	gridInventory = append(gridInventory, input.AllPriorOutputs()...)

	training.grid.Update(gridInventory)
	training.retain(gridInventory)
	training.extendSignature(gridInventory)
	reading := training.predict(scratch, false)
	record, err := training.detector.Observe(scratch)

	if err != nil {
		training.Error(err)
	}

	if record != nil {
		record.Epoch = training.epoch
		training.resolve(scratch.Label, record)
	}

	training.publish(gridInventory, scratch, output, record, reading, false)
	training.nudge()
}

func (training *Training) retain(measurements []*data.Measurement[float64]) {
	if len(measurements) == 0 {
		return
	}

	label := measurements[0].Label
	if label == "" {
		return
	}

	training.mu.Lock()
	for _, m := range measurements {
		if m != nil {
			training.frames[label] = append(training.frames[label], m.Clone())
		}
	}
	training.mu.Unlock()
}

func (training *Training) extendSignature(measurements []*data.Measurement[float64]) {
	if len(measurements) == 0 {
		return
	}

	label := measurements[0].Label
	if label == "" || !training.ready() {
		return
	}

	token := training.grid.LitRegions(measurements)

	if len(token) == 0 {
		return
	}

	training.mu.Lock()
	training.signatures[label] = appendLitFrame(
		append([]byte{}, training.signatures[label]...),
		token,
	)
	training.mu.Unlock()
}

func (training *Training) resolve(symbol string, record *tables.ExcursionRecord) {
	training.mu.Lock()
	frames := cloneFrames(framesBefore(training.frames[symbol], record.ExitTick))
	training.frames[symbol] = framesFrom(training.frames[symbol], record.ExitTick)
	training.signatures[symbol] = training.signatureOf(training.frames[symbol])
	training.mu.Unlock()

	episode := heldEpisode{record: *record, frames: frames}

	if err := training.enqueue(episode); err != nil {
		training.Error(err)
	}
}

func (training *Training) predict(measurement *data.Measurement[float64], historical bool) marker {
	if measurement == nil || measurement.Label == "" || !training.ready() {
		return marker{}
	}

	training.mu.Lock()
	signature := append([]byte{}, training.signatures[measurement.Label]...)
	training.mu.Unlock()

	return training.predictFrom(signature, measurement.Label, measurement.SeqIdx, historical)
}

func (training *Training) predictFrom(signature []byte, symbol string, seq int64, historical bool) marker {
	if len(signature) == 0 {
		return marker{}
	}

	result, err := training.engine.Evaluate(signature)

	if err != nil {
		training.Error(err)

		return marker{}
	}

	action := cognition.Action(result.Evaluation.WinnerClass)

	// Historical replay has no live inventory. Surface ENTER/EXIT markers for
	// training viz without requiring trader holding — otherwise LegalActions
	// (holding=false → enter/wait only) drops every EXIT leaf from the stream.
	if historical {
		if action == cognition.ActionEnter {
			return marker{action: string(action), entry: seq}
		}

		if action == cognition.ActionExit {
			return marker{action: string(action), exit: seq}
		}

		return marker{}
	}

	holding := training.holding(symbol)

	if !admitted(holding, action) {
		return marker{}
	}

	if action == cognition.ActionEnter && training.busy(symbol) {
		return marker{}
	}

	if action == cognition.ActionEnter && !training.paperOpen() {
		training.mu.Lock()
		training.paperPredictions++
		training.mu.Unlock()

		return marker{action: string(action), entry: seq}
	}

	if action == cognition.ActionEnter {
		training.remember(symbol, signature)
		training.mu.Lock()
		training.paperPredictions++
		training.mu.Unlock()

		if training.trader != nil {
			if err := training.trader.OnAction(symbol, action, result.Evaluation.Confidence); err != nil {
				// Soft-fail: insufficient available cash / below-min / paper
				// place validation — skip enter, keep training READY (same
				// spirit as RemapConservative soft-fail).
				if broker.IsEnterSoftFail(err) {
					errnie.Warn("[training] paper enter skipped: " + err.Error())
					training.forget(symbol)

					return marker{}
				}

				training.Error(err)

				return marker{action: string(action), entry: seq}
			}
		}

		training.mu.Lock()
		training.paperTrades++
		training.mu.Unlock()

		return marker{action: string(action), entry: seq}
	}

	// Paper feedback is owned solely by onPositionClosed after a reconciled
	// fill — never grade synchronously on the EXIT decision path.
	if training.trader != nil {
		if err := training.trader.OnAction(symbol, action, result.Evaluation.Confidence); err != nil {
			training.Error(err)

			return marker{action: string(action), exit: seq}
		}
	}

	return marker{action: string(action), exit: seq}
}

/*
onPositionClosed is the sole owner of paper feedback. Position close after a
reconciled fill grades once; the EXIT decision path never grades synchronously.
*/
func (training *Training) onPositionClosed(symbol string, regulator *position.Regulator) {
	if training == nil || symbol == "" || regulator == nil {
		return
	}

	training.mu.Lock()
	entryCtx := append([]byte{}, training.entries[symbol]...)
	training.mu.Unlock()

	training.gradePaperClosed(symbol, entryCtx, regulator)

	if regulator.IsClosed() {
		training.forget(symbol)
	}
}

/*
gradePaperClosed scores the remembered enter context against a reconciled exit.
Forward paper return is realized PnL / economic basis (fraction), never ±1 sign.
One reconciled closed position => exactly one learning update.
*/
func (training *Training) gradePaperClosed(
	symbol string,
	entryCtx []byte,
	regulator *position.Regulator,
) {
	if len(entryCtx) == 0 || regulator == nil || !regulator.IsClosed() {
		return
	}

	if regulator.Realized == nil {
		return
	}

	basis := regulator.ClosedCost()
	if basis == nil || basis.Sign() <= 0 {
		return
	}

	denom := basis.Float64()
	if !finiteFloat(denom) || denom == 0 {
		return
	}

	feedback := regulator.Realized.Float64() / denom
	if !finiteFloat(feedback) {
		return
	}

	key := symbol + "\x00" + string(entryCtx)
	training.mu.Lock()
	if training.paperGraded[key] {
		training.mu.Unlock()
		return
	}
	training.paperGraded[key] = true
	replaying := training.replaying
	if replaying {
		training.deferredPaper = append(training.deferredPaper, paperGrade{
			symbol:   symbol,
			entryCtx: append([]byte{}, entryCtx...),
			feedback: feedback,
		})
		training.mu.Unlock()
		return
	}
	training.mu.Unlock()

	training.applyPaperGrade(symbol, entryCtx, feedback)
}

func (training *Training) applyPaperGrade(symbol string, entryCtx []byte, feedback float64) {
	if len(entryCtx) == 0 || !finiteFloat(feedback) {
		return
	}

	training.teach(entryCtx, string(cognition.ActionEnter), feedback)

	training.mu.Lock()
	training.paper.Update(feedback)
	training.mu.Unlock()
}


/*
supervise freezes model decisions BEFORE outcomes are used, then scores the
economic result of THAT causal policy with executable bid/ask + fees.

Causal policy edge (historical):
  - A→B freeze ENTER? open hypo at executable ask+fee
  - B→C freeze EXIT? close at executable bid+fee at that decision
  - ENTER but never EXIT before C → incomplete policy return 0 (NOT oracle C)
C's liquidation (execReturn) is delayed supervision for teach only.

Abstain (non-Enter) = zero policy return. edge / historical returns come only
from these frozen model-policy returns — never raw episode ProfitFraction.
Prediction accuracy (skill ±1) stays separate: prediction grades; ground truth
teaches. ENTER and EXIT are always taught from resolved C when wantEnter
(breaks the bootstrap deadlock of teaching only if already predicted).
Fee-failing ups stay silent; down/chop/flat teach ENTER avoidance (-1).
One episode ID is taught/scored once; A-offset augmentation is the explicit
exception (replayVaried).
*/
func (training *Training) supervise(episode heldEpisode, score bool) {
	training.mu.Lock()
	if training.graded[episode.record.ID] {
		training.mu.Unlock()
		return
	}
	if score {
		training.graded[episode.record.ID] = true
		training.countFragmentLocked(episode.record.Direction)
	}
	training.mu.Unlock()

	record := episode.record
	// Enter: A→B (precursor developing into ignition).
	enterCtx := training.signatureOf(framesBefore(episode.frames, record.AnchorTick))
	// Exit: B→C (ignition developing into exhaustion) — not the full A→C path.
	exitCtx := training.signatureOf(framesRange(episode.frames, record.AnchorTick, record.ExitTick))
	wantEnter := record.Direction == "up" && record.ClearsFriction
	execReturn := executableEnterReturn(record)

	// FREEZE model decisions before outcome-dependent teaching/scoring.
	var predicted cognition.Action
	if len(enterCtx) > 0 {
		predicted = training.frozenAction(enterCtx)
	}
	var predictedExit cognition.Action
	if len(exitCtx) > 0 {
		predictedExit = training.frozenAction(exitCtx)
	}

	// Causal policy edge: only a completed ENTER→EXIT hypo earns execReturn.
	// Missed exit at C is penalized with actual forced liquidation return at C (not 0.0!).
	policyReturn := 0.0

	if predicted == cognition.ActionEnter && predictedExit == cognition.ActionExit {
		policyReturn = execReturn
	}

	if predicted == cognition.ActionEnter && predictedExit != cognition.ActionExit {
		policyReturn = forcedEndReturn(record)
	}

	if score {
		training.mu.Lock()
		training.recordPolicyReturnLocked(policyReturn, record)
		training.mu.Unlock()
	}

	if len(enterCtx) > 0 {
		// Ground truth teaches ENTER; prediction grades only.
		// Fee-clearing ups: economic execReturn. Fee-failing ups stay silent
		// (detector may have chopped a longer move). Down/chop/flat: ENTER -1
		// so avoidance has honest negatives.
		if wantEnter {
			training.teach(enterCtx, string(cognition.ActionEnter), execReturn)
		}

		if !wantEnter && record.Direction != "up" {
			training.teach(enterCtx, string(cognition.ActionEnter), -1)
		}

		if score {
			correct := (wantEnter && predicted == cognition.ActionEnter) ||
				(!wantEnter && predicted != cognition.ActionEnter)
			training.recordSkill(correct)
			training.noteEnterGrade(wantEnter, predicted == cognition.ActionEnter)
		}
	}

	// Ground truth controls learning: always teach EXIT from resolved C when
	// wantEnter, regardless of frozen prediction (prediction grades only).
	if wantEnter && len(exitCtx) > 0 {
		training.teach(exitCtx, string(cognition.ActionExit), execReturn)

		if score {
			training.recordSkill(predictedExit == cognition.ActionExit)
			training.noteExitGrade(predictedExit == cognition.ActionExit)
		}
	}
}

/*
forcedEndReturn computes the economic return of an un-exited trade liquidated
at confirmed structural reversal C (PostEndTick / PostEndPrice).
*/
func forcedEndReturn(record tables.ExcursionRecord) float64 {
	if !finiteFloat(record.EntryPrice) || record.EntryPrice <= 0 {
		return 0
	}

	endPrice := record.PostEndPrice

	if !finiteFloat(endPrice) || endPrice <= 0 {
		endPrice = record.ExitPrice
	}

	if !finiteFloat(endPrice) {
		return 0
	}

	feeRate := record.Fee

	if feeRate <= 0 {
		feeRate = 0.0026
	}

	netProceeds := endPrice * (1.0 - feeRate)
	fraction := (netProceeds - record.EntryPrice) / record.EntryPrice

	if !finiteFloat(fraction) {
		return 0
	}

	return fraction
}

/*
executableEnterReturn is the economic fraction of buying at the episode's
executable entry (ask+fee) and selling at its executable exit (bid-fee).
Uses stored EntryPrice/ExitPrice produced by the Detector's bid/ask+fee path.
*/
func executableEnterReturn(record tables.ExcursionRecord) float64 {
	if !finiteFloat(record.EntryPrice) || record.EntryPrice <= 0 {
		return 0
	}
	if !finiteFloat(record.ExitPrice) {
		return 0
	}
	fraction := (record.ExitPrice - record.EntryPrice) / record.EntryPrice
	if !finiteFloat(fraction) {
		return 0
	}
	return fraction
}

func (training *Training) frozenAction(signature []byte) cognition.Action {
	if len(signature) == 0 {
		return ""
	}

	result, err := training.engine.Evaluate(signature)

	if err != nil {
		training.Error(err)

		return ""
	}

	return cognition.Action(result.Evaluation.WinnerClass)
}

func (training *Training) teach(context []byte, class string, feedback float64) {
	if len(context) == 0 || class == "" || class == string(cognition.ActionWait) {
		return
	}

	_, err := training.engine.Observe(cognition.Association{
		Context:  append([]byte{}, context...),
		Class:    []byte(class),
		Feedback: feedback,
		Graded:   true,
	})

	if err != nil {
		training.Error(err)
		return
	}

	training.mu.Lock()
	training.modelRevision++
	training.mu.Unlock()

	select {
	case training.settleWake <- struct{}{}:
	default:
	}
}

func (training *Training) recordSkill(correct bool) {
	sample := -1.0

	if correct {
		sample = 1
	}

	training.mu.Lock()
	training.skill.Update(sample)
	training.skillSamples = append(training.skillSamples, sample)
	const skillSampleCap = 256
	if len(training.skillSamples) > skillSampleCap {
		training.skillSamples = append([]float64(nil), training.skillSamples[len(training.skillSamples)-skillSampleCap:]...)
	}
	// Skill is part of the durable checkpoint; bump revision so save() persists.
	training.modelRevision++
	training.mu.Unlock()

	select {
	case training.settleWake <- struct{}{}:
	default:
	}
}

/*
recordPolicyReturnLocked accumulates frozen model-policy returns for edge /
hist_mean_return. Caller holds mu. Abstain contributes 0. Skill ±1 correctness
is separate — never publish it as bp. Never records raw ProfitFraction.
*/
func (training *Training) recordPolicyReturnLocked(policyReturn float64, record tables.ExcursionRecord) {
	if !finiteFloat(policyReturn) {
		return
	}

	training.returns.Update(policyReturn)
	training.returnSamples = append(training.returnSamples, policyReturn)
	const returnSampleCap = 256
	if len(training.returnSamples) > returnSampleCap {
		training.returnSamples = append([]float64(nil), training.returnSamples[len(training.returnSamples)-returnSampleCap:]...)
	}

	span := float64(record.ExitTick - record.AnchorTick)
	if span > 0 {
		training.fragmentSpanSum += span
	}

	if record.ClearsFriction {
		training.histClears++
	}

	if !record.ClearsFriction && record.Direction == "up" {
		training.histFeeFailUp++
	}

	training.modelRevision++
}

func (training *Training) replayHistory() error {
	if training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: catalog is required for historical replay",
			nil,
		))
	}

	durable, err := training.catalog.Excursions(training.Context(), training.epoch, nil)

	if err != nil {
		return err
	}

	tapeRows, err := training.loadQuoteTape()

	if err != nil {
		return err
	}

	isolatedPrice := training.isolatedPrice()
	canonical, offErr := DetectExcursions(NewDetector(isolatedPrice), tapeRows)

	if offErr != nil {
		return offErr
	}

	excursions, persistIDs := ReconcileExcursions(canonical, durable)

	if len(excursions) == 0 {
		return nil
	}

	bySymbol := tapeBySymbol(canonicalObservations(tapeRows))

	return training.replayCausal(excursions, bySymbol, persistIDs)
}

func (training *Training) isolatedPrice() *broker.Price {
	ctx := training.Context()
	isolatedBook := broker.NewBook(ctx, nil)
	price := broker.NewPrice(ctx, isolatedBook, nil, nil, nil)

	if training.price != nil {
		price.SetReferenceCash(training.price.ReferenceCash())
		training.price.CopyFeesTo(price)
	}

	return price
}

/*
replayCausal walks every excursion frame and outcome on one chronological
event stream so supervise(C) cannot run before predict(B) on an earlier tick.
Durable Iceberg records are supervised only — never re-enqueued for persist.
*/
func (training *Training) replayCausal(
	excursions []tables.ExcursionRecord,
	bySymbol map[string][]*data.Measurement[float64],
	persistIDs map[string]bool,
) error {
	type replayEvent struct {
		seq   int64
		phase int // 0 = frame (predict), 1 = outcome (supervise)
		idx   int
		frame *data.Measurement[float64]
	}

	events := make([]replayEvent, 0)

	for index := range excursions {
		record := &excursions[index]

		if record.ID == "" || record.Symbol == "" {
			continue
		}

		if record.Epoch <= 0 {
			record.Epoch = training.epoch
		}

		start := record.PrecursorStartTick
		end := record.ExitTick

		if record.PostEndTick > end {
			end = record.PostEndTick
		}

		for _, measurement := range bySymbol[record.Symbol] {
			if measurement == nil {
				continue
			}

			if measurement.SeqIdx < start || measurement.SeqIdx > end {
				continue
			}

			events = append(events, replayEvent{
				seq:   measurement.SeqIdx,
				phase: 0,
				idx:   index,
				frame: measurement,
			})
		}

		// Outcome is observable at ExitTick — never earlier.
		events = append(events, replayEvent{
			seq:   record.ExitTick,
			phase: 1,
			idx:   index,
		})
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].seq != events[j].seq {
			return events[i].seq < events[j].seq
		}

		if events[i].phase != events[j].phase {
			return events[i].phase < events[j].phase
		}

		return events[i].idx < events[j].idx
	})

	type openState struct {
		signature []byte
		frames    []*data.Measurement[float64]
	}

	states := make([]openState, len(excursions))

	for _, event := range events {
		select {
		case <-training.Context().Done():
			return training.Context().Err()
		default:
		}

		record := &excursions[event.idx]

		if event.phase == 0 {
			if event.frame == nil {
				continue
			}

			clone := event.frame.Clone()
			inventory := []*data.Measurement[float64]{clone}
			training.grid.Update(inventory)

			if clone.SeqIdx < record.ExitTick {
				states[event.idx].frames = append(states[event.idx].frames, clone)
			}

			token := training.grid.LitRegions(inventory)
			states[event.idx].signature = appendLitFrame(states[event.idx].signature, token)
			reading := training.predictFrom(states[event.idx].signature, clone.Label, clone.SeqIdx, true)
			training.publish(inventory, clone, clone, record, reading, true)

			continue
		}

		if len(states[event.idx].frames) == 0 {
			continue
		}

		episode := heldEpisode{
			record: *record,
			frames: cloneFrames(states[event.idx].frames),
		}

		if persistIDs[record.ID] {
			// Persist C then introduce supervision BEFORE advancing the replay
			// clock — async enqueue would let later predictions race Iceberg.
			if err := training.persistAndSupervise(episode); err != nil {
				return err
			}

			continue
		}

		// Already durable in Iceberg: teach when the outcome tick is reached —
		// never re-persist the same ExcursionRecord.
		training.acceptHistorical(episode)
	}

	return training.waitDrained()
}

/*
acceptHistorical supervises a durable Iceberg episode without enqueue→persist.
Before the grid is checkpointed, park on pending like live noteDurable.
*/
func (training *Training) acceptHistorical(episode heldEpisode) {
	training.mu.Lock()
	checkpointed := training.checkpointed

	if !checkpointed {
		training.pending = append(training.pending, episode)
		training.mu.Unlock()

		return
	}

	training.episodes = append(training.episodes, episode)
	training.mu.Unlock()
	training.supervise(episode, true)

	select {
	case training.episodeReady <- struct{}{}:
	default:
	}
}

/*
loadQuoteTape loads venue ticker rows and generated measurements for the epoch.
Ticker carries bid/ask for Detector; measurements carry sync SeqIdx overlays.
*/
func (training *Training) loadQuoteTape() ([]*data.Measurement[float64], error) {
	ticker, err := training.catalog.Collect(training.Context(), tables.SpotTicker, training.epoch)

	if err != nil {
		return nil, err
	}

	measured, err := training.catalog.Collect(training.Context(), tables.Measurements, training.epoch)

	if err != nil {
		return nil, err
	}

	level3, err := training.catalog.Collect(training.Context(), tables.SpotLevel3, training.epoch)

	if err != nil && !errnie.IsNotFound(err) {
		return nil, err
	}

	return append(append(ticker, measured...), level3...), nil
}

/*
replayExcursion walks one fragment for tests and offline tooling. persist=false
accepts durable Iceberg records without re-writing them; persist=true enqueues
new offline detections only.
*/
func (training *Training) replayExcursion(
	record *tables.ExcursionRecord,
	tape []*data.Measurement[float64],
	persist bool,
) error {
	if record == nil || record.ID == "" || record.Symbol == "" {
		return nil
	}

	persistIDs := map[string]bool{}

	if persist {
		persistIDs[record.ID] = true
	}

	return training.replayCausal([]tables.ExcursionRecord{*record}, map[string][]*data.Measurement[float64]{
		record.Symbol: tape,
	}, persistIDs)
}

/*
tapeBySymbol groups canonical observations by label for excursion windows.
*/
func tapeBySymbol(rows []*data.Measurement[float64]) map[string][]*data.Measurement[float64] {
	bySymbol := make(map[string][]*data.Measurement[float64])

	for _, row := range rows {
		if row == nil || row.Label == "" {
			continue
		}

		bySymbol[row.Label] = append(bySymbol[row.Label], row)
	}

	return bySymbol
}

func (training *Training) augment() {
	for {
		select {
		case <-training.Context().Done():
			return
		case <-training.episodeReady:
		}

		training.mu.Lock()
		episodes := append([]heldEpisode{}, training.episodes...)
		training.mu.Unlock()

		for _, episode := range episodes {
			if training.Context().Err() != nil {
				return
			}

			training.replayVaried(episode)
		}
	}
}

/*
replayVaried teaches one unused A-offset precursor per call. Keys are
episodeID + frame index so recognition variation cannot re-count the same
evidence forever while the augment loop runs.
*/
func (training *Training) replayVaried(episode heldEpisode) {
	var eligible []int

	for index, frame := range episode.frames {
		if frame != nil && frame.SeqIdx < episode.record.AnchorTick {
			eligible = append(eligible, index)
		}
	}

	if len(eligible) == 0 {
		return
	}

	training.mu.Lock()
	var unused []int

	for _, index := range eligible {
		key := episode.record.ID + "\x00" + strconv.Itoa(index)

		if !training.augmented[key] {
			unused = append(unused, index)
		}
	}

	if len(unused) == 0 {
		training.mu.Unlock()

		return
	}

	pick := unused[rand.IntN(len(unused))]
	training.augmented[episode.record.ID+"\x00"+strconv.Itoa(pick)] = true
	training.mu.Unlock()

	enterCtx := training.signatureOf(episode.frames[:pick+1])
	wantEnter := episode.record.Direction == "up" && episode.record.ClearsFriction
	execReturn := executableEnterReturn(episode.record)

	// A-offset augmentation is the explicit exception to one-shot teach: still
	// uses executable economic feedback, never ±1 correctness.
	if wantEnter {
		training.teach(enterCtx, string(cognition.ActionEnter), execReturn)
	}

	if !wantEnter && episode.record.Direction != "up" {
		training.teach(enterCtx, string(cognition.ActionEnter), execReturn)
	}
}

/*
persistAndSupervise writes one offline C to Iceberg then supervises it before
the caller advances the causal replay clock.
*/
func (training *Training) persistAndSupervise(episode heldEpisode) error {
	if training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: catalog is required to store an episode",
			nil,
		))
	}

	writer := tables.NewWriter(training.catalog, training.epoch)
	writer.AddExcursion(episode.record)
	commitCtx, cancel := context.WithTimeout(training.Context(), persistCommitTimeout)
	err := training.commitExcursionsBounded(commitCtx, writer)
	cancel()
	if err != nil {
		return err
	}

	// Historical offline C: supervise immediately on the causal clock. Do not
	// route through noteDurable — that defers live learning during replay.
	training.acceptHistorical(episode)
	return nil
}

/*
commitExcursionsBounded runs CommitExcursions but surfaces ctx cancellation to
the UI before Append returns. Iceberg Append may ignore cancellation after the
commitGate is held; without this, writing stays true forever and the learning
UI latches on "durability blocked: writing".
*/
func (training *Training) commitExcursionsBounded(ctx context.Context, writer *tables.Writer) error {
	if writer == nil {
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- writer.CommitExcursions(ctx)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Prefer a precise persistErr over "writing" while the orphan Append
		// still holds the catalog commitGate.
		training.mu.Lock()
		training.persistErr = errnie.Error(errnie.Err(
			errnie.Timeout,
			"training: excursion commit exceeded "+persistCommitTimeout.String(),
			ctx.Err(),
		))
		training.cond.Broadcast()
		training.mu.Unlock()

		err := <-done
		if err != nil {
			return err
		}
		// Append finished successfully after the deadline — treat as success so
		// finishWrite / noteDurable still run; persistErr is cleared there.
		return nil
	}
}

func (training *Training) enqueue(episode heldEpisode) error {
	if training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: catalog is required to store an episode",
			nil,
		))
	}

	training.mu.Lock()
	training.queue = append(training.queue, episode)
	training.blocked = true
	// Keep persistErr while an orphan Append still holds writing; clearing it
	// here made stage() fall back to "durability blocked: writing" forever.
	if !training.writing {
		training.persistErr = nil
	}
	training.mu.Unlock()

	select {
	case training.wake <- struct{}{}:
	default:
	}

	return nil
}

func (training *Training) persistLoop() {
	if training.catalog == nil {
		training.failPersist(errnie.Error(errnie.Err(
			errnie.Validation,
			"training: catalog is required to store an episode",
			nil,
		)))

		return
	}

	backoff := time.Second

	for {
		select {
		case <-training.Context().Done():
			training.mu.Lock()
			training.cond.Broadcast()
			training.mu.Unlock()

			return
		case <-training.wake:
		}

		for {
			training.mu.Lock()
			batch := training.queue
			training.queue = nil
			training.writing = len(batch) > 0
			training.blocked = training.writing
			training.mu.Unlock()

			if len(batch) == 0 {
				break
			}

			// Fresh Writer per attempt. CommitExcursions restores uncommitted
			// rows into the same Writer on failure; re-AddExcursion on a reused
			// Writer would duplicate Iceberg appends after a transient error.
			writer := tables.NewWriter(training.catalog, training.epoch)

			for _, episode := range batch {
				writer.AddExcursion(episode.record)
			}

			commitCtx, cancel := context.WithTimeout(training.Context(), persistCommitTimeout)
			err := training.commitExcursionsBounded(commitCtx, writer)
			cancel()
			training.finishWrite(batch, err)

			if err == nil {
				backoff = time.Second
				continue
			}

			if training.Context().Err() != nil {
				return
			}

			// Do not wait for a new enqueue wake: a failed commit left
			// the batch requeued and blocked=true with nothing else waking us.
			select {
			case <-training.Context().Done():
				return
			case <-time.After(backoff):
			}

			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

func (training *Training) finishWrite(batch []heldEpisode, err error) {
	if err != nil {
		training.mu.Lock()
		training.queue = append(batch, training.queue...)
		training.writing = false
		training.blocked = true
		training.persistErr = err
		training.cond.Broadcast()
		training.mu.Unlock()
		training.failPersist(err)

		return
	}

	for _, episode := range batch {
		training.noteDurable(episode)
	}

	training.mu.Lock()
	training.writing = false
	training.persistErr = nil
	training.blocked = len(training.queue) > 0
	training.persistFailed = false
	training.cond.Broadcast()
	training.mu.Unlock()
}

func (training *Training) noteDurable(episode heldEpisode) {
	training.mu.Lock()
	checkpointed := training.checkpointed

	if !checkpointed {
		training.pending = append(training.pending, episode)
		training.mu.Unlock()

		return
	}

	// Live persist during historical replay: keep the Iceberg write, queue
	// cognition teaching until replay completes (replay isolation).
	if training.replaying {
		training.deferredLearn = append(training.deferredLearn, episode)
		training.mu.Unlock()
		return
	}

	training.episodes = append(training.episodes, episode)
	training.mu.Unlock()
	training.supervise(episode, true)

	select {
	case training.episodeReady <- struct{}{}:
	default:
	}
}

func (training *Training) waitDrained() error {
	training.mu.Lock()
	defer training.mu.Unlock()

	// Persist failures requeue and retry in persistLoop; do not bail on the
	// first persistErr or historical replay aborts while durability stays blocked.
	for len(training.queue) > 0 || training.writing {
		if err := training.Context().Err(); err != nil {
			return err
		}

		training.cond.Wait()
	}

	return training.persistErr
}

func (training *Training) checkpointLoop() {
	for {
		if training.Context().Err() != nil {
			return
		}

		training.mu.Lock()
		restored := training.restored
		checkpointed := training.checkpointed
		rev := training.modelRevision
		snap := training.snapshotRevision
		training.mu.Unlock()

		if !restored {
			missing, err := training.restore()

			if err != nil {
				training.failCheckpoint(err)
				training.waitSettle()

				continue
			}

			training.mu.Lock()
			training.restored = true
			training.mu.Unlock()

			// TRAINING.md: grid settles once, then the trie is regularly
			// checkpointed. Restoring a settled snapshot makes us durable but
			// must not exit the loop — later teach() dirties the engine.
			if !missing && training.grid.IsSettled() {
				training.markDurable()
				training.waitSettle()

				continue
			}
		}

		if training.grid.IsSettled() && !checkpointed {
			if err := training.save(); err != nil {
				training.failCheckpoint(err)
				training.waitSettle()

				continue
			}

			training.markDurable()
			training.waitSettle()

			continue
		}

		// Persist trie updates after each graded teach once the grid is frozen.
		// Monotonic revision: save only when snapshot lags; never clear a newer dirty.
		if checkpointed && snap < rev {
			if err := training.save(); err != nil {
				training.failCheckpoint(err)
				training.waitSettle()

				continue
			}

			training.mu.Lock()
			if training.modelRevision == rev {
				training.snapshotRevision = rev
			}
			training.checkpointFailed = false
			training.mu.Unlock()
		}

		training.waitSettle()
	}
}

func (training *Training) waitSettle() {
	select {
	case <-training.Context().Done():
	case <-training.settleWake:
	}
}

func (training *Training) restore() (bool, error) {
	if training.catalog == nil {
		return true, nil
	}

	encoded, err := training.catalog.GetBlob(training.Context(), gridKey)

	if errors.Is(err, tables.ErrBlobMissing) {
		return true, nil
	}

	if err != nil {
		return false, err
	}

	if err = training.grid.RestoreSnapshot(encoded); err != nil {
		return false, err
	}

	model, err := training.catalog.GetBlob(training.Context(), engineKey)

	if errors.Is(err, tables.ErrBlobMissing) {
		training.resetCognitionLocked()
		return true, nil
	}

	if err != nil {
		return false, err
	}

	if _, err = training.engine.Restore(model); err != nil {
		// packed-weight/1 and other old formats: refuse contamination.
		training.resetCognitionLocked()
		return true, nil
	}

	skillBlob, skillErr := training.catalog.GetBlob(training.Context(), skillKey)

	if skillErr != nil && !errors.Is(skillErr, tables.ErrBlobMissing) {
		return false, skillErr
	}

	if skillErr == nil {
		if err = training.applySkillCheckpoint(skillBlob); err != nil {
			// Old supervisionCheckpointVersion: start uncontaminated.
			training.resetCognitionLocked()
			return true, nil
		}
	}

	return false, nil
}

// resetCognitionLocked drops restored grid/engine/skill so an experiment under
// new supervision rules cannot inherit contaminated state. Caller need not hold mu.
func (training *Training) resetCognitionLocked() {
	training.grid = store.NewGrid()
	training.engine = cognition.NewEngine(cognition.Config{})
	training.mu.Lock()
	training.skill = statistic.Moments{}
	training.returns = statistic.Moments{}
	training.skillSamples = nil
	training.returnSamples = nil
	training.histCorrectEnter = 0
	training.histMissedEnter = 0
	training.histFalseEnter = 0
	training.histCorrectExit = 0
	training.histMissedExit = 0
	training.histCorrectWait = 0
	training.histClears = 0
	training.histFeeFailUp = 0
	training.fragmentSpanSum = 0
	training.mu.Unlock()
}

func (training *Training) save() error {
	if training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: catalog is required to checkpoint",
			nil,
		))
	}

	encoded, err := training.grid.Snapshot()

	if err != nil {
		return err
	}

	model, err := training.engine.Snapshot()

	if err != nil {
		return err
	}

	if err = training.catalog.PutBlob(training.Context(), gridKey, encoded); err != nil {
		return err
	}

	if err = training.catalog.PutBlob(training.Context(), engineKey, model.Model); err != nil {
		return err
	}

	skillBlob, err := training.skillCheckpoint()

	if err != nil {
		return err
	}

	return training.catalog.PutBlob(training.Context(), skillKey, skillBlob)
}

/*
skillCheckpoint encodes observed skill / policy-return moments and hist grade
counters for durable restore. Values are exactly what recordSkill /
noteEnterGrade / recordPolicyReturn accumulated — never invented.
*/
type skillCheckpoint struct {
	Version          int               `json:"version"`
	Skill            statistic.Moments `json:"skill"`
	Returns          statistic.Moments `json:"returns,omitempty"`
	ReturnSamples    []float64         `json:"return_samples,omitempty"`
	HistCorrectEnter float64           `json:"hist_correct_enter,omitempty"`
	HistMissedEnter  float64           `json:"hist_missed_enter,omitempty"`
	HistFalseEnter   float64           `json:"hist_false_enter,omitempty"`
	HistCorrectExit  float64           `json:"hist_correct_exit,omitempty"`
	HistMissedExit   float64           `json:"hist_missed_exit,omitempty"`
	HistCorrectWait  float64           `json:"hist_correct_wait,omitempty"`
	HistClears       float64           `json:"hist_clears,omitempty"`
	HistFeeFailUp    float64           `json:"hist_fee_fail_up,omitempty"`
	FragmentSpanSum  float64           `json:"fragment_span_sum,omitempty"`
}

func (training *Training) skillCheckpoint() ([]byte, error) {
	training.mu.Lock()
	payload := skillCheckpoint{
		Version:          supervisionCheckpointVersion,
		Skill:            training.skill,
		Returns:          training.returns,
		ReturnSamples:    append([]float64(nil), training.returnSamples...),
		HistCorrectEnter: training.histCorrectEnter,
		HistMissedEnter:  training.histMissedEnter,
		HistFalseEnter:   training.histFalseEnter,
		HistCorrectExit:  training.histCorrectExit,
		HistMissedExit:   training.histMissedExit,
		HistCorrectWait:  training.histCorrectWait,
		HistClears:       training.histClears,
		HistFeeFailUp:    training.histFeeFailUp,
		FragmentSpanSum:  training.fragmentSpanSum,
	}
	training.mu.Unlock()

	encoded, err := json.Marshal(payload)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"training: encode skill checkpoint",
			err,
		))
	}

	return encoded, nil
}

func (training *Training) applySkillCheckpoint(encoded []byte) error {
	var payload skillCheckpoint

	if err := json.Unmarshal(encoded, &payload); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"training: decode skill checkpoint",
			err,
		))
	}

	if payload.Version != supervisionCheckpointVersion {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: refuse skill checkpoint from old supervision rules",
			nil,
		))
	}

	// Reject empty/partial garbage without inventing samples.
	if payload.Skill.Count < 0 || math.IsNaN(payload.Skill.Mean) || math.IsNaN(payload.Skill.M2) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: skill checkpoint moments are invalid",
			nil,
		))
	}

	if payload.Returns.Count < 0 || math.IsNaN(payload.Returns.Mean) || math.IsNaN(payload.Returns.M2) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: returns checkpoint moments are invalid",
			nil,
		))
	}

	if payload.HistCorrectEnter < 0 || payload.HistMissedEnter < 0 || payload.HistFalseEnter < 0 ||
		payload.HistCorrectExit < 0 || payload.HistMissedExit < 0 || payload.HistCorrectWait < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: hist grade counters in skill checkpoint are invalid",
			nil,
		))
	}

	training.mu.Lock()
	training.skill = payload.Skill
	training.returns = payload.Returns
	training.returnSamples = append([]float64(nil), payload.ReturnSamples...)
	training.histCorrectEnter = payload.HistCorrectEnter
	training.histMissedEnter = payload.HistMissedEnter
	training.histFalseEnter = payload.HistFalseEnter
	training.histCorrectExit = payload.HistCorrectExit
	training.histMissedExit = payload.HistMissedExit
	training.histCorrectWait = payload.HistCorrectWait
	training.histClears = payload.HistClears
	training.histFeeFailUp = payload.HistFeeFailUp
	training.fragmentSpanSum = payload.FragmentSpanSum
	training.mu.Unlock()

	return nil
}

func (training *Training) markDurable() {
	training.mu.Lock()
	training.checkpointed = true
	training.checkpointFailed = false
	pending := training.pending
	training.pending = nil
	training.episodes = append(training.episodes, pending...)
	training.rebuildLocked()
	training.mu.Unlock()
	training.durableOnce.Do(func() { close(training.durable) })

	for _, episode := range pending {
		training.supervise(episode, true)
	}
}

func (training *Training) rebuildLocked() {
	for symbol, frames := range training.frames {
		training.signatures[symbol] = training.signatureOf(frames)
	}
}

func (training *Training) nudge() {
	if !training.grid.IsSettled() || training.checkpointed {
		return
	}

	select {
	case training.settleWake <- struct{}{}:
	default:
	}
}

func (training *Training) ready() bool {
	training.mu.Lock()
	defer training.mu.Unlock()

	return training.checkpointed
}

func (training *Training) paperOpen() bool {
	training.mu.Lock()
	moments := training.skill
	blocked := training.blocked
	checkpointed := training.checkpointed
	replaying := training.replaying
	training.mu.Unlock()

	if blocked || replaying || !checkpointed || moments.Count <= 1 {
		return false
	}

	dispersion := math.Sqrt(moments.M2 / (moments.Count - 1))
	lower := moments.Mean - dispersion/math.Sqrt(moments.Count)

	return lower > 0
}

func (training *Training) stage() (float64, string, string) {
	training.mu.Lock()
	checkpointed := training.checkpointed
	blocked := training.blocked
	writing := training.writing
	persistErr := training.persistErr
	replaying := training.replaying
	moments := training.skill
	training.mu.Unlock()

	if !training.grid.IsSettled() {
		return 0, "MODEL DEVELOPMENT", "grid unsettled"
	}

	if !checkpointed {
		return 0, "MODEL DEVELOPMENT", "checkpoint missing"
	}

	// Durability blockers outrank "replay in progress" so a hung Iceberg
	// Append cannot hide behind HISTORICAL VALIDATION with an empty reason.
	if blocked {
		detail := "durability blocked: pending"

		if writing {
			detail = "durability blocked: writing"
		}

		if persistErr != nil {
			detail = "durability blocked: " + persistErr.Error()
		}

		return 1, "HISTORICAL VALIDATION", detail
	}

	if replaying {
		return 1, "HISTORICAL VALIDATION", "historical replay in progress"
	}

	if moments.Count == 0 {
		return 1, "HISTORICAL VALIDATION", "no graded skill outcomes yet"
	}

	if moments.Count <= 1 {
		return 1, "HISTORICAL VALIDATION", "skill samples=" + strconv.FormatFloat(moments.Count, 'f', 0, 64) + " (need >1 for lower bound)"
	}

	dispersion := math.Sqrt(moments.M2 / (moments.Count - 1))
	lower := moments.Mean - dispersion/math.Sqrt(moments.Count)

	if lower <= 0 {
		return 1, "HISTORICAL VALIDATION", "skill lower bound not positive (samples=" + strconv.FormatFloat(moments.Count, 'f', 0, 64) + ")"
	}

	return 2, "FORWARD PAPER LEARNING", ""
}

func (training *Training) publish(
	gridInventory []*data.Measurement[float64],
	scratch *data.Measurement[float64],
	output *data.Measurement[float64],
	record *tables.ExcursionRecord,
	reading marker,
	historical bool,
) {
	if training.tee == nil || output == nil {
		return
	}

	output.SetSource("training:live")

	if historical {
		output.SetSource("training:historical")
	}

	code, name, blocker := training.stage()

	if historical {
		code = 1
		name = "HISTORICAL VALIDATION"
		// Keep durability / skill-gate blockers visible. Only drop the noisy
		// in-progress label so frames still show why the gate is closed.
		if blocker == "historical replay in progress" {
			blocker = ""
		}
	}

	output.WriteMetric("stage_code", code)
	output.SetProvenance("stage", name)
	output.SetProvenance("stage_blocker", blocker)
	training.writeSkill(output)
	training.writeQuote(output, scratch)
	training.writeMarker(output, reading, gridInventory)
	training.writeEpisode(output, record)

	// Live open excursions: stamp A/B from Detector while C is still unknown.
	if record == nil && !historical && output.Label != "" && training.detector != nil {
		if precursor, anchor, open := training.detector.OpenMarks(output.Label); open {
			output.SetMetadata("excursion_event", "open")
			output.SetMetadata("excursion_start", strconv.FormatInt(precursor, 10))
			output.SetMetadata("excursion_ignition", strconv.FormatInt(anchor, 10))
			output.WriteMetric("mark_a", float64(precursor))
			output.WriteMetric("mark_b", float64(anchor))
		}
	}

	training.tee.Push(output)
}

func (training *Training) writeSkill(clone *data.Measurement[float64]) {
	training.mu.Lock()
	skill := training.skill
	returns := training.returns
	paper := training.paper
	trades := training.paperTrades
	predictions := training.paperPredictions
	up := training.fragmentsUp
	down := training.fragmentsDown
	chop := training.fragmentsChop
	flat := training.fragmentsFlat
	unsup := training.fragmentsUnsupported
	clears := training.histClears
	feeFail := training.histFeeFailUp
	spanSum := training.fragmentSpanSum
	correctEnter := training.histCorrectEnter
	missedEnter := training.histMissedEnter
	falseEnter := training.histFalseEnter
	correctExit := training.histCorrectExit
	missedExit := training.histMissedExit
	correctWait := training.histCorrectWait
	samples := append([]float64(nil), training.returnSamples...)
	training.mu.Unlock()

	// hist_mean_return / edge are frozen model-policy return means (abstain=0).
	// skill.Mean is ±1 correctness — never publish it as bp.
	if returns.Count > 0 {
		clone.WriteMetric("hist_opportunities", returns.Count)
		clone.WriteMetric("hist_mean_return", returns.Mean)
		if returns.Count > 1 {
			dispersion := math.Sqrt(returns.M2 / (returns.Count - 1))
			clone.WriteMetric("hist_lower_bound", returns.Mean-dispersion/math.Sqrt(returns.Count))
		}
		clone.WriteMetric("edge", returns.Mean)
	}

	clone.WriteMetric("hist_correct_enter", correctEnter)
	clone.WriteMetric("hist_missed_enter", missedEnter)
	clone.WriteMetric("hist_false_enter", falseEnter)
	clone.WriteMetric("hist_correct_exit", correctExit)
	clone.WriteMetric("hist_missed_exit", missedExit)
	clone.WriteMetric("hist_correct_wait", correctWait)
	clone.WriteMetric("fragments_clears", clears)
	clone.WriteMetric("fragments_fee_fail_up", feeFail)
	if returns.Count > 0 && spanSum > 0 {
		clone.WriteMetric("fragment_mean_ticks", spanSum/returns.Count)
	}

	if len(samples) > 0 {
		parts := make([]string, len(samples))
		for index, value := range samples {
			parts[index] = strconv.FormatFloat(value, 'f', -1, 64)
		}
		clone.SetMetadata("edge_samples", strings.Join(parts, ","))
		clone.WriteMetric("edge_sample_count", float64(len(samples)))
	}

	clone.WriteMetric("fragments_up", up)
	clone.WriteMetric("fragments_down", down)
	clone.WriteMetric("fragments_chop", chop)
	clone.WriteMetric("fragments_flat", flat)
	clone.WriteMetric("fragments_unsupported", unsup)

	if trades > 0 {
		clone.WriteMetric("fwd_paper_trades", trades)
		clone.WriteMetric("fwd_enter_predictions", predictions)
	}

	if paper.Count > 1 {
		dispersion := math.Sqrt(paper.M2 / (paper.Count - 1))
		clone.WriteMetric("fwd_paper_mean_return", paper.Mean)
		clone.WriteMetric("fwd_paper_lower_bound", paper.Mean-dispersion/math.Sqrt(paper.Count))
	}

	// win_rate / accuracy = skill accuracy from ±1 grades. edge (above) is
	// economic mean policy return so UI basis (×10000) is honest bp.
	clone.WriteMetric("resolved", skill.Count)
	if training.engine != nil {
		clone.WriteMetric("steps", float64(training.engine.Step()))
		clone.WriteMetric("decisions", float64(training.engine.Len()))
	}
	if skill.Count > 0 {
		winRate := (skill.Mean + 1) / 2
		clone.WriteMetric("win_rate", winRate)
		clone.WriteMetric("accuracy", winRate)
	}
}

func (training *Training) writeQuote(clone *data.Measurement[float64], measurement *data.Measurement[float64]) {
	seen, ok := quoteFrom(measurement)

	if ok {
		clone.WriteMetric("price", seen.mid)
		return
	}

	// Honest fallback when bid/ask quote is incomplete: mid/last/close/price
	// already on the tape — never invent a level.
	if price, ok := observedTapePrice(measurement); ok {
		clone.WriteMetric("price", price)
	}
}

/*
observedTapePrice reads an already-measured price level from the frame.
Prefers an explicit mid, then last/close/price, then bid+ask mid when both
sides exist. Single-sided quotes are refused (not a mid).
*/
func observedTapePrice(measurement *data.Measurement[float64]) (float64, bool) {
	if measurement == nil {
		return 0, false
	}

	for _, name := range []string{"mid", "last", "last_price", "close", "price", "mark_price", "trade_price"} {
		if value, ok := lookupPositiveMetric(measurement, name); ok {
			return value, true
		}
	}

	bid, bidOK := lookupPositiveMetric(measurement, "bid")
	ask, askOK := lookupPositiveMetric(measurement, "ask")

	if bidOK && askOK && ask >= bid {
		return (bid + ask) / 2, true
	}

	return 0, false
}

func lookupPositiveMetric(measurement *data.Measurement[float64], name string) (float64, bool) {
	metric, ok := measurement.LookupMetric(name)

	if !ok {
		return 0, false
	}

	value := metric.Raw

	if metric.Exact != nil {
		value = metric.Exact.Float64()
	}

	if !positiveFinite(value) {
		return 0, false
	}

	return value, true
}

/*
countFragmentLocked tallies a graded ExcursionRecord direction. Caller holds mu.
*/
func (training *Training) countFragmentLocked(direction string) {
	switch direction {
	case "up":
		training.fragmentsUp++
	case "down":
		training.fragmentsDown++
	case "chop":
		training.fragmentsChop++
	case "flat":
		training.fragmentsFlat++
	default:
		training.fragmentsUnsupported++
	}
}

/*
noteEnterGrade records whether a frozen precursor Evaluate matched the enter
requirement of a graded excursion — the same decision recordSkill uses.
*/
func (training *Training) noteEnterGrade(wantEnter, predictedEnter bool) {
	training.mu.Lock()
	defer training.mu.Unlock()

	if wantEnter && predictedEnter {
		training.histCorrectEnter++
		return
	}

	if wantEnter && !predictedEnter {
		training.histMissedEnter++
		return
	}

	if !wantEnter && predictedEnter {
		training.histFalseEnter++
		return
	}

	training.histCorrectWait++
}

func (training *Training) noteExitGrade(correct bool) {
	training.mu.Lock()
	defer training.mu.Unlock()

	if correct {
		training.histCorrectExit++
		return
	}

	training.histMissedExit++
}

func (training *Training) writeMarker(
	clone *data.Measurement[float64],
	reading marker,
	measurements []*data.Measurement[float64],
) {
	if reading.action == string(cognition.ActionEnter) {
		clone.WriteMetric("frozen_prediction", 1)
		clone.WriteMetric("action", 1)
	}

	if reading.action == string(cognition.ActionExit) {
		clone.WriteMetric("frozen_prediction", 2)
		clone.WriteMetric("action", 2)
	}

	if reading.entry > 0 {
		clone.WriteMetric("agent_entry", float64(reading.entry))
	}

	if reading.exit > 0 {
		clone.WriteMetric("agent_exit", float64(reading.exit))
	}

	// LitRegions must use the observed measurement (pre Source rewrite). The
	// published clone is stamped training:* for UI routing; that Source never
	// matched grid cells when Source was part of the key, and even after the
	// cellKey fix the clone may carry overlay-only metrics.
	token := training.grid.LitRegions(measurements)

	if len(token) == 0 {
		clone.WriteMetric("precursor_length", 0)
		return
	}

	parts := make([]string, len(token))

	for index, value := range token {
		parts[index] = strconv.Itoa(int(value))
	}

	clone.WriteMetric("precursor_length", float64(len(token)))
	clone.SetMetadata("precursor_tokens", strings.Join(parts, ","))
	clone.SetProvenance("precursor_tokens", strings.Join(parts, ","))
}

func (training *Training) writeEpisode(clone *data.Measurement[float64], record *tables.ExcursionRecord) {
	if record == nil {
		return
	}

	clone.SetMetadata("excursion_direction", record.Direction)
	clone.SetMetadata("excursion_clears", strconv.FormatBool(record.ClearsFriction))
	clone.SetMetadata("excursion_start", strconv.FormatInt(record.PrecursorStartTick, 10))
	clone.SetMetadata("excursion_ignition", strconv.FormatInt(record.AnchorTick, 10))
	clone.SetMetadata("excursion_exit", strconv.FormatInt(record.ExitTick, 10))

	event := "developing"
	if clone.SeqIdx >= record.ExitTick {
		event = "completed"
	}
	clone.SetMetadata("excursion_event", event)

	clone.WriteMetric("mark_a", float64(record.PrecursorStartTick))
	clone.WriteMetric("mark_b", float64(record.AnchorTick))
	clone.WriteMetric("mark_c", float64(record.ExitTick))
	clone.WriteMetric("excursion_mag", record.ProfitFraction)
}

func (training *Training) signatureOf(frames []*data.Measurement[float64]) []byte {
	var signature []byte

	for _, frame := range frames {
		signature = appendLitFrame(signature, training.grid.LitRegions([]*data.Measurement[float64]{frame}))
	}

	return signature
}

func (training *Training) holding(symbol string) bool {
	regulator := training.position(symbol)

	return regulator != nil && regulator.IsHolding()
}

func (training *Training) busy(symbol string) bool {
	regulator := training.position(symbol)

	return regulator != nil && !regulator.IsClosed()
}

func (training *Training) position(symbol string) *position.Regulator {
	if training.trader == nil {
		return nil
	}

	return training.trader.Position(symbol)
}

func (training *Training) remember(symbol string, signature []byte) {
	training.mu.Lock()
	training.entries[symbol] = append([]byte{}, signature...)
	training.mu.Unlock()
}

func (training *Training) forget(symbol string) {
	training.mu.Lock()
	delete(training.entries, symbol)
	training.mu.Unlock()
}

func (training *Training) failCheckpoint(err error) {
	training.mu.Lock()
	failed := training.checkpointFailed
	training.checkpointFailed = true
	training.mu.Unlock()

	if !failed {
		training.Error(err)
	}
}

func (training *Training) failPersist(err error) {
	training.mu.Lock()
	failed := training.persistFailed
	training.persistFailed = true
	training.mu.Unlock()

	if !failed {
		training.Error(err)
	}
}

func admitted(holding bool, action cognition.Action) bool {
	if action != cognition.ActionEnter && action != cognition.ActionExit {
		return false
	}

	for _, legal := range cognition.LegalActions(holding) {
		if legal == action {
			return true
		}
	}

	return false
}

func framesBefore(frames []*data.Measurement[float64], tick int64) []*data.Measurement[float64] {
	chosen := make([]*data.Measurement[float64], 0)

	for _, frame := range frames {
		if frame != nil && frame.SeqIdx < tick {
			chosen = append(chosen, frame)
		}
	}

	return chosen
}

/*
framesRange keeps [start, end) by SeqIdx — B→C exit learning uses
AnchorTick inclusive through ExitTick exclusive.
*/
func framesRange(frames []*data.Measurement[float64], start, end int64) []*data.Measurement[float64] {
	chosen := make([]*data.Measurement[float64], 0)

	for _, frame := range frames {
		if frame == nil {
			continue
		}

		if frame.SeqIdx >= start && frame.SeqIdx < end {
			chosen = append(chosen, frame)
		}
	}

	return chosen
}

func framesFrom(frames []*data.Measurement[float64], tick int64) []*data.Measurement[float64] {
	chosen := make([]*data.Measurement[float64], 0)

	for _, frame := range frames {
		if frame != nil && frame.SeqIdx >= tick {
			chosen = append(chosen, frame)
		}
	}

	return chosen
}

func cloneFrames(frames []*data.Measurement[float64]) []*data.Measurement[float64] {
	copied := make([]*data.Measurement[float64], 0, len(frames))

	for _, frame := range frames {
		if frame != nil {
			copied = append(copied, frame.Clone())
		}
	}

	return copied
}

/*
canonicalObservations rebuilds the live disruptor observation per workspace
sequence: websocket ingress as root + deterministic Source-keyed producer peers.
training:* overlays are excluded. Same representation Live Training.Step,
Grid.Update, LitRegions, and historical replay consume — no richest-row heuristic.
*/
func canonicalObservations(rows []*data.Measurement[float64]) []*data.Measurement[float64] {
	type key struct {
		label string
		seq   int64
	}

	groups := make(map[key][]*data.Measurement[float64])
	order := make([]key, 0)

	for _, row := range rows {
		if row == nil || row.Label == "" {
			continue
		}
		if strings.HasPrefix(row.Source, "training:") {
			continue
		}
		k := key{label: row.Label, seq: row.SeqIdx}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], row)
	}

	sort.SliceStable(order, func(i, j int) bool {
		if order[i].label != order[j].label {
			return order[i].label < order[j].label
		}
		return order[i].seq < order[j].seq
	})

	kept := make([]*data.Measurement[float64], 0, len(order))
	for _, k := range order {
		obs := assembleCanonical(groups[k])
		if obs != nil {
			kept = append(kept, obs)
		}
	}
	return kept
}

func assembleCanonical(rows []*data.Measurement[float64]) *data.Measurement[float64] {
	if len(rows) == 0 {
		return nil
	}

	var ingress *data.Measurement[float64]
	peers := make([]*data.Measurement[float64], 0, len(rows))

	for _, row := range rows {
		if row == nil {
			continue
		}
		if isIngressSource(row) {
			if ingress == nil {
				ingress = row.Clone()
				ingress.Peers = nil
			}
			continue
		}
		peers = append(peers, row)
	}

	if ingress == nil {
		// No raw websocket row for this seq — synthesize ingress shell from the
		// first producer so Label/SeqIdx/provenance match live shared-slot identity.
		seed := rows[0]
		for _, row := range rows {
			if row != nil {
				seed = row
				break
			}
		}
		if seed == nil {
			return nil
		}
		ingress = data.NewMeasurement[float64]("websocket", nil)
		ingress.Label = seed.Label
		ingress.SeqIdx = seed.SeqIdx
		ingress.At = seed.At
		if ch, ok := seed.GetProvenance("ingress_channel"); ok {
			ingress.SetProvenance("ingress_channel", ch)
		}
		if ch, ok := seed.GetProvenance("channel"); ok {
			ingress.SetProvenance("channel", ch)
		}
	}

	// Deterministic Contribute order by Source then SeqIdx (Contribute also sorts).
	sort.SliceStable(peers, func(i, j int) bool {
		if peers[i].Source != peers[j].Source {
			return peers[i].Source < peers[j].Source
		}
		return peers[i].SeqIdx < peers[j].SeqIdx
	})

	for _, peer := range peers {
		owned := peer.Clone()
		owned.Peers = nil
		ingress.Contribute(owned)
	}

	return ingress
}

func isIngressSource(row *data.Measurement[float64]) bool {
	if row == nil {
		return false
	}
	if row.Source == "websocket" {
		return true
	}
	// Legacy venue rows may keep channel provenance with empty/rewritten Source.
	if row.Source == "" {
		if ch, ok := row.GetProvenance("ingress_channel"); ok {
			switch ch {
			case "ticker", "trade", "level3", "futures_ticker", "futures_trade":
				return true
			}
		}
	}
	return false
}
