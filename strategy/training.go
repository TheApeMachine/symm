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
	skillSamples         []float64
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
	durable              chan struct{}
	durableOnce          sync.Once
	wake                 chan struct{}
	settleWake           chan struct{}
	episodeReady         chan struct{}
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

func (training *Training) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if measurement == nil {
		return nil
	}

	training.live(measurement)

	return measurement
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
		training.mu.Unlock()

		if err != nil {
			training.Error(err)

			return
		}

		training.augment()
	}()
}

func (training *Training) live(measurement *data.Measurement[float64]) {
	training.grid.Update(measurement)
	training.retain(measurement)
	training.extendSignature(measurement)
	reading := training.predict(measurement, false)
	record, err := training.detector.Observe(measurement)

	if err != nil {
		training.Error(err)
	}

	if record != nil {
		record.Epoch = training.epoch
		training.resolve(measurement.Label, record)
	}

	training.publish(measurement, record, reading, false)
	training.nudge()
}

func (training *Training) retain(measurement *data.Measurement[float64]) {
	if measurement.Label == "" {
		return
	}

	training.mu.Lock()
	training.frames[measurement.Label] = append(training.frames[measurement.Label], measurement.Clone())
	training.mu.Unlock()
}

func (training *Training) extendSignature(measurement *data.Measurement[float64]) {
	if measurement.Label == "" || !training.ready() {
		return
	}

	token := training.grid.LitRegions(measurement)

	if len(token) == 0 {
		return
	}

	training.mu.Lock()
	training.signatures[measurement.Label] = appendLitFrame(
		append([]byte{}, training.signatures[measurement.Label]...),
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
				training.Error(err)

				return marker{action: string(action), entry: seq}
			}
		}

		training.mu.Lock()
		training.paperTrades++
		training.mu.Unlock()

		return marker{action: string(action), entry: seq}
	}

	// Grade paper only after EXIT is submitted and the position has reconciled.
	// Mark-to-market PnL before Exit still prices an open inventory and forgets
	// the entry context before the fill exists (review P0).
	training.mu.Lock()
	entryCtx := append([]byte{}, training.entries[symbol]...)
	training.mu.Unlock()

	var closed *position.Regulator

	if training.trader != nil {
		closed = training.trader.Position(symbol)

		if err := training.trader.OnAction(symbol, action, result.Evaluation.Confidence); err != nil {
			training.Error(err)

			return marker{action: string(action), exit: seq}
		}
	}

	training.gradePaperClosed(symbol, entryCtx, closed)

	// Forget only after a reconciled closed grade — dropping entryCtx while
	// the exit is still pending loses the teachable precursor.
	if closed != nil && closed.IsClosed() {
		training.forget(symbol)
	}

	return marker{action: string(action), exit: seq}
}

/*
onPositionClosed grades and forgets when a regulator closes via ApplyExecution
(async fill path). Sync Exit that already closed still grades in the decision path.
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
Requires a closed regulator with Realized PnL — open mark-to-market is not an
episode outcome.
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

	feedback := 0.0

	if regulator.Realized.Sign() > 0 {
		feedback = 1
	}

	if regulator.Realized.Sign() < 0 {
		feedback = -1
	}

	if feedback == 0 {
		return
	}

	// Teach the enter association from realized outcome; paper moments track
	// forward proof. Historical skill is scored separately in supervise.
	training.teach(entryCtx, string(cognition.ActionEnter), feedback)

	training.mu.Lock()
	training.paper.Update(feedback)
	training.mu.Unlock()
}

/*
supervise teaches the trie from ground-truth excursion outcomes and, when
score is set, updates skill from whether a frozen Evaluate of the precursor
matched what the outcome required — not from profitability alone.
*/
func (training *Training) supervise(episode heldEpisode, score bool) {
	training.mu.Lock()

	if training.graded[episode.record.ID] {
		score = false
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

	if len(enterCtx) > 0 {
		predicted := training.frozenAction(enterCtx)

		if wantEnter {
			training.teach(enterCtx, string(cognition.ActionEnter), 1)
		} else {
			training.teach(enterCtx, string(cognition.ActionEnter), -1)
		}

		if score {
			correct := (wantEnter && predicted == cognition.ActionEnter) ||
				(!wantEnter && predicted != cognition.ActionEnter)
			training.recordSkill(correct)
			training.noteEnterGrade(wantEnter, predicted == cognition.ActionEnter)
		}
	}

	if wantEnter && len(exitCtx) > 0 {
		predicted := training.frozenAction(exitCtx)
		training.teach(exitCtx, string(cognition.ActionExit), 1)

		if score {
			training.recordSkill(predicted == cognition.ActionExit)
			training.noteExitGrade(predicted == cognition.ActionExit)
		}
	}
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

func (training *Training) replayHistory() error {
	if training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: catalog is required for historical replay",
			nil,
		))
	}

	// Stored ExcursionRecords are the episode owners for replay (no live
	// Detector re-walk inventing a second timeline on the same marks).
	// After-the-fact detection may *add* records from the persisted quote
	// tape (ticker/measurements + SeqIdx sync) using the same Detector+fees.
	durable, err := training.catalog.Excursions(training.Context(), training.epoch, nil)

	if err != nil {
		return err
	}

	tapeRows, err := training.loadQuoteTape()

	if err != nil {
		return err
	}

	offline, offErr := training.detectOffline(tapeRows, durable)

	if offErr != nil {
		return offErr
	}

	persistIDs := make(map[string]bool, len(offline))

	for index := range offline {
		persistIDs[offline[index].ID] = true
	}

	excursions := append(append([]tables.ExcursionRecord{}, durable...), offline...)

	if len(excursions) == 0 {
		return nil
	}

	bySymbol := tapeBySymbol(preferRows(tapeRows))

	return training.replayCausal(excursions, bySymbol, persistIDs)
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
			training.grid.Update(clone)

			if clone.SeqIdx < record.ExitTick {
				states[event.idx].frames = append(states[event.idx].frames, clone)
			}

			token := training.grid.LitRegions(clone)
			states[event.idx].signature = appendLitFrame(states[event.idx].signature, token)
			reading := training.predictFrom(states[event.idx].signature, clone.Label, clone.SeqIdx, true)
			training.publish(clone, record, reading, true)

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

	return append(ticker, measured...), nil
}

/*
detectOffline runs Detector over the stored quote tape and enqueues records
whose IDs are not already in Iceberg — after-the-fact detection alongside live.
*/
func (training *Training) detectOffline(
	tape []*data.Measurement[float64],
	existing []tables.ExcursionRecord,
) ([]tables.ExcursionRecord, error) {
	if training.price == nil || len(tape) == 0 {
		return nil, nil
	}

	known := make(map[string]bool, len(existing))

	for index := range existing {
		known[existing[index].ID] = true
	}

	detected, err := DetectExcursions(NewDetector(training.price), tape)

	if err != nil {
		return nil, err
	}

	added := make([]tables.ExcursionRecord, 0)

	for index := range detected {
		record := detected[index]

		if known[record.ID] {
			continue
		}

		if record.Epoch <= 0 {
			record.Epoch = training.epoch
		}

		// Returned to replayCausal with persistIDs — do not enqueue frameless.
		known[record.ID] = true
		added = append(added, record)
	}

	return added, nil
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
tapeBySymbol groups preferred measurement rows by label for excursion windows.
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
	feedback := -1.0

	if wantEnter {
		feedback = 1
	}

	training.teach(enterCtx, string(cognition.ActionEnter), feedback)
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
	err := writer.CommitExcursions(commitCtx)
	cancel()
	if err != nil {
		return err
	}

	training.noteDurable(episode)
	return nil
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
	training.persistErr = nil
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
			err := writer.CommitExcursions(commitCtx)
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

	if err != nil {
		return false, err
	}

	if _, err = training.engine.Restore(model); err != nil {
		return false, err
	}

	skillBlob, skillErr := training.catalog.GetBlob(training.Context(), skillKey)

	if skillErr != nil && !errors.Is(skillErr, tables.ErrBlobMissing) {
		return false, skillErr
	}

	if skillErr == nil {
		if err = training.applySkillCheckpoint(skillBlob); err != nil {
			return false, err
		}
	}

	return false, nil
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
skillCheckpoint encodes the observed historical skill moments for durable
restore. Values are exactly what recordSkill accumulated — never invented.
*/
type skillCheckpoint struct {
	Skill statistic.Moments `json:"skill"`
}

func (training *Training) skillCheckpoint() ([]byte, error) {
	training.mu.Lock()
	payload := skillCheckpoint{Skill: training.skill}
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

	// Reject empty/partial garbage without inventing samples.
	if payload.Skill.Count < 0 || math.IsNaN(payload.Skill.Mean) || math.IsNaN(payload.Skill.M2) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: skill checkpoint moments are invalid",
			nil,
		))
	}

	training.mu.Lock()
	training.skill = payload.Skill
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

	if replaying {
		return 1, "HISTORICAL VALIDATION", "historical replay in progress"
	}

	if blocked {
		detail := "durability blocked"
		if persistErr != nil {
			detail = "durability blocked: " + persistErr.Error()
		} else if writing {
			detail = "durability blocked: writing"
		} else {
			detail = "durability blocked: pending"
		}
		return 1, "HISTORICAL VALIDATION", detail
	}

	if moments.Count <= 1 {
		return 1, "HISTORICAL VALIDATION", "skill lower bound not positive"
	}

	dispersion := math.Sqrt(moments.M2 / (moments.Count - 1))
	lower := moments.Mean - dispersion/math.Sqrt(moments.Count)

	if lower <= 0 {
		return 1, "HISTORICAL VALIDATION", "skill lower bound not positive"
	}

	return 2, "FORWARD PAPER LEARNING", ""
}

func (training *Training) publish(
	measurement *data.Measurement[float64],
	record *tables.ExcursionRecord,
	reading marker,
	historical bool,
) {
	if training.tee == nil || measurement == nil {
		return
	}

	clone := measurement.Clone()
	clone.SetSource("training:live")

	if historical {
		clone.SetSource("training:historical")
	}

	code, name, blocker := training.stage()

	if historical {
		code = 1
		name = "HISTORICAL VALIDATION"
		blocker = ""
	}

	clone.WriteMetric("stage_code", code)
	clone.SetProvenance("stage", name)
	clone.SetProvenance("stage_blocker", blocker)
	training.writeSkill(clone)
	training.writeQuote(clone, measurement)
	training.writeMarker(clone, reading, measurement)
	training.writeEpisode(clone, record)

	// Live open excursions: stamp A/B from Detector while C is still unknown.
	if record == nil && !historical && measurement.Label != "" && training.detector != nil {
		if precursor, anchor, open := training.detector.OpenMarks(measurement.Label); open {
			clone.SetMetadata("excursion_event", "open")
			clone.SetMetadata("excursion_start", strconv.FormatInt(precursor, 10))
			clone.SetMetadata("excursion_ignition", strconv.FormatInt(anchor, 10))
			clone.WriteMetric("mark_a", float64(precursor))
			clone.WriteMetric("mark_b", float64(anchor))
		}
	}

	training.tee.Push(clone)
}

func (training *Training) writeSkill(clone *data.Measurement[float64]) {
	training.mu.Lock()
	skill := training.skill
	paper := training.paper
	trades := training.paperTrades
	predictions := training.paperPredictions
	up := training.fragmentsUp
	down := training.fragmentsDown
	chop := training.fragmentsChop
	flat := training.fragmentsFlat
	unsup := training.fragmentsUnsupported
	correctEnter := training.histCorrectEnter
	missedEnter := training.histMissedEnter
	falseEnter := training.histFalseEnter
	correctExit := training.histCorrectExit
	missedExit := training.histMissedExit
	correctWait := training.histCorrectWait
	samples := append([]float64(nil), training.skillSamples...)
	training.mu.Unlock()

	if skill.Count > 1 {
		dispersion := math.Sqrt(skill.M2 / (skill.Count - 1))
		clone.WriteMetric("hist_opportunities", skill.Count)
		clone.WriteMetric("hist_mean_return", skill.Mean)
		clone.WriteMetric("hist_lower_bound", skill.Mean-dispersion/math.Sqrt(skill.Count))
	}

	clone.WriteMetric("hist_correct_enter", correctEnter)
	clone.WriteMetric("hist_missed_enter", missedEnter)
	clone.WriteMetric("hist_false_enter", falseEnter)
	clone.WriteMetric("hist_correct_exit", correctExit)
	clone.WriteMetric("hist_missed_exit", missedExit)
	clone.WriteMetric("hist_correct_wait", correctWait)

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
	observed *data.Measurement[float64],
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
	token := training.grid.LitRegions(observed)

	if len(token) == 0 {
		return
	}

	parts := make([]string, len(token))

	for index, value := range token {
		parts[index] = strconv.Itoa(int(value))
	}

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
		signature = appendLitFrame(signature, training.grid.LitRegions(frame))
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

func preferRows(rows []*data.Measurement[float64]) []*data.Measurement[float64] {
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Label != rows[right].Label {
			return rows[left].Label < rows[right].Label
		}

		if rows[left].SeqIdx != rows[right].SeqIdx {
			return rows[left].SeqIdx < rows[right].SeqIdx
		}

		// training:* overlays are UI publications (stage_code, marks), never
		// tape fragments — exclude them even if they carry extra overlay metrics.
		leftTrain := strings.HasPrefix(rows[left].Source, "training:")
		rightTrain := strings.HasPrefix(rows[right].Source, "training:")

		if leftTrain != rightTrain {
			return !leftTrain
		}

		// Keep the richest observation for this symbol/seq. Concurrent stage
		// consumers push the shared slot at different mutation depths; raw
		// websocket ingress cannot light frozen regions the way Training.Step
		// saw them.
		leftN := metricCount(rows[left])
		rightN := metricCount(rows[right])

		if leftN != rightN {
			return leftN > rightN
		}

		return sourceRank(rows[left].Source) < sourceRank(rows[right].Source)
	})

	kept := make([]*data.Measurement[float64], 0, len(rows))

	for _, row := range rows {
		if len(kept) > 0 && kept[len(kept)-1].Label == row.Label && kept[len(kept)-1].SeqIdx == row.SeqIdx {
			continue
		}

		kept = append(kept, row)
	}

	return kept
}

func metricCount(measurement *data.Measurement[float64]) int {
	if measurement == nil {
		return 0
	}

	seen := make(map[string]struct{}, len(measurement.Metrics))
	for key := range measurement.Metrics {
		seen[key] = struct{}{}
	}

	for _, peer := range measurement.Peers {
		if peer == nil {
			continue
		}

		for key := range peer.Metrics {
			seen[key] = struct{}{}
		}
	}

	return len(seen)
}

func sourceRank(source string) int {
	if strings.HasPrefix(source, "training:") {
		return 100
	}

	if source == "websocket" {
		return 50
	}

	return 0
}
