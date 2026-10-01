package strategy

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"

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
	augmented        map[string]bool
	skill            statistic.Moments
	paper            statistic.Moments
	paperTrades      float64
	paperPredictions float64
	checkpointed     bool
	checkpointFailed bool
	modelDirty       bool
	restored         bool
	blocked          bool
	writing          bool
	persistFailed    bool
	persistErr       error
	durable          chan struct{}
	durableOnce      sync.Once
	wake             chan struct{}
	settleWake       chan struct{}
	episodeReady     chan struct{}
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

		if err := training.replayHistory(); err != nil {
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
	signature := append(append([]byte{}, training.signatures[measurement.Label]...), token...)
	training.signatures[measurement.Label] = append(signature, 0)
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
	holding := training.holding(symbol)

	if !admitted(holding, action) {
		return marker{}
	}

	if action == cognition.ActionEnter && training.busy(symbol) {
		return marker{}
	}

	if action == cognition.ActionEnter && historical {
		return marker{action: string(action), entry: seq}
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

	if historical {
		return marker{action: string(action), exit: seq}
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
	training.forget(symbol)

	return marker{action: string(action), exit: seq}
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
	}

	training.mu.Unlock()

	record := episode.record
	enterCtx := training.signatureOf(framesBefore(episode.frames, record.AnchorTick))
	exitCtx := training.signatureOf(framesBefore(episode.frames, record.ExitTick))
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
		}
	}

	if wantEnter && len(exitCtx) > 0 {
		predicted := training.frozenAction(exitCtx)
		training.teach(exitCtx, string(cognition.ActionExit), 1)

		if score {
			training.recordSkill(predicted == cognition.ActionExit)
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
	training.modelDirty = true
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
	training.mu.Unlock()
}

func (training *Training) replayHistory() error {
	if training.catalog == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"training: catalog is required for historical replay",
			nil,
		))
	}

	// ExcursionRecord is the sole historical episode owner (TRAINING.md ground
	// truth). Do not re-detect from Measurements — that invents a second episode
	// timeline. Live Step still uses Detector for newly forming excursions.
	excursions, err := training.catalog.Excursions(training.Context(), training.epoch, nil)

	if err != nil {
		return err
	}

	if len(excursions) == 0 {
		return nil
	}

	rows, err := training.catalog.Collect(training.Context(), tables.Measurements, training.epoch)

	if err != nil {
		return err
	}

	bySymbol := tapeBySymbol(preferRows(rows))

	for index := range excursions {
		select {
		case <-training.Context().Done():
			return training.Context().Err()
		default:
		}

		if err := training.replayExcursion(&excursions[index], bySymbol[excursions[index].Symbol]); err != nil {
			return err
		}
	}

	return training.waitDrained()
}

/*
replayExcursion walks the stored tape fragment for one ExcursionRecord through
the frozen grid, publishes learning frames, and enqueues that record as the
episode — never a Detector re-observation.
*/
func (training *Training) replayExcursion(
	record *tables.ExcursionRecord,
	tape []*data.Measurement[float64],
) error {
	if record == nil || record.ID == "" || record.Symbol == "" {
		return nil
	}

	if record.Epoch <= 0 {
		record.Epoch = training.epoch
	}

	start := record.PrecursorStartTick
	end := record.ExitTick

	if record.PostEndTick > end {
		end = record.PostEndTick
	}

	frames := make([]*data.Measurement[float64], 0)
	signature := make([]byte, 0)

	for _, measurement := range tape {
		if measurement == nil {
			continue
		}

		if measurement.SeqIdx < start || measurement.SeqIdx > end {
			continue
		}

		clone := measurement.Clone()
		training.grid.Update(clone)

		if clone.SeqIdx < record.ExitTick {
			frames = append(frames, clone)
		}

		token := training.grid.LitRegions(clone)

		if len(token) > 0 {
			signature = append(signature, token...)
			signature = append(signature, 0)
		}

		reading := training.predictFrom(signature, clone.Label, clone.SeqIdx, true)
		var published *tables.ExcursionRecord

		if clone.SeqIdx == record.ExitTick {
			published = record
		}

		training.publish(clone, published, reading, true)
	}

	if len(frames) == 0 {
		return nil
	}

	return training.enqueue(heldEpisode{record: *record, frames: cloneFrames(frames)})
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

	writer := tables.NewWriter(training.catalog, training.epoch)

	for {
		select {
		case <-training.Context().Done():
			training.mu.Lock()
			training.cond.Broadcast()
			training.mu.Unlock()

			return
		case <-training.wake:
		}

		training.mu.Lock()
		batch := training.queue
		training.queue = nil
		training.writing = len(batch) > 0
		training.blocked = training.writing
		training.mu.Unlock()

		if len(batch) == 0 {
			continue
		}

		for _, episode := range batch {
			writer.AddExcursion(episode.record)
		}

		err := writer.CommitReady(training.Context(), true)
		training.finishWrite(batch, err)
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

	for len(training.queue) > 0 || training.writing {
		if training.persistErr != nil && !training.writing {
			return training.persistErr
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
		dirty := training.modelDirty
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
			if !missing && training.grid.Settled {
				training.markDurable()
				training.waitSettle()

				continue
			}
		}

		if training.grid.Settled && !checkpointed {
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
		if checkpointed && dirty {
			if err := training.save(); err != nil {
				training.failCheckpoint(err)
				training.waitSettle()

				continue
			}

			training.mu.Lock()
			training.modelDirty = false
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

	return training.catalog.PutBlob(training.Context(), engineKey, model.Model)
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
	if !training.grid.Settled || training.checkpointed {
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
	training.mu.Unlock()

	if blocked || !checkpointed || moments.Count <= 1 {
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
	moments := training.skill
	training.mu.Unlock()

	if !training.grid.Settled {
		return 0, "MODEL DEVELOPMENT", "grid unsettled"
	}

	if !checkpointed {
		return 0, "MODEL DEVELOPMENT", "checkpoint missing"
	}

	if blocked {
		return 1, "HISTORICAL VALIDATION", "durability blocked"
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
	training.tee.Push(clone)
}

func (training *Training) writeSkill(clone *data.Measurement[float64]) {
	training.mu.Lock()
	skill := training.skill
	paper := training.paper
	trades := training.paperTrades
	predictions := training.paperPredictions
	training.mu.Unlock()

	if skill.Count > 1 {
		dispersion := math.Sqrt(skill.M2 / (skill.Count - 1))
		clone.WriteMetric("hist_opportunities", skill.Count)
		clone.WriteMetric("hist_mean_return", skill.Mean)
		clone.WriteMetric("hist_lower_bound", skill.Mean-dispersion/math.Sqrt(skill.Count))
	}

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

	if !ok {
		return
	}

	clone.WriteMetric("price", seen.mid)
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
	clone.SetMetadata("excursion_event", "completed")
	clone.WriteMetric("mark_a", float64(record.PrecursorStartTick))
	clone.WriteMetric("mark_b", float64(record.AnchorTick))
	clone.WriteMetric("mark_c", float64(record.ExitTick))
	clone.WriteMetric("excursion_mag", record.ProfitFraction)
}

func (training *Training) signatureOf(frames []*data.Measurement[float64]) []byte {
	var signature []byte

	for _, frame := range frames {
		token := training.grid.LitRegions(frame)

		if len(token) == 0 {
			continue
		}

		signature = append(signature, token...)
		signature = append(signature, 0)
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

	return len(measurement.Metrics)
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
