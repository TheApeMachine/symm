package strategy

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	rand "math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/ui"
	"golang.design/x/lockfree/lf"
)

const (
	actionEnter = "enter"
	actionExit  = "exit"
	actionWait  = "wait"

	// fragmentRehearsals is how many times Train walks every stored excursion
	// with a fresh random A offset before the trie is frozen for paper trading.
	// TRAINING.md: loop fragments with A offset before B to harden precursors.
	fragmentRehearsals = 4
)

var (
	_ ui.CognitionSource = (*Training)(nil)
	_ ui.FragmentsSource = (*Training)(nil)
)

func stringLess(a, b string) bool {
	return a < b
}

/*
Training develops one grid, loads the trie from historical excursions, then
paper trades the live market with it. Realized round trips refine the trie.
*/
type Training struct {
	*runtime.System
	arena        *data.ArenaOwner
	grid         *store.Grid
	memory       *cognition.Associate
	recall       *cognition.Recall
	trainer      *cognition.Train
	census       *cognition.Census
	snapshot     *cognition.Snapshot
	tree         *cognition.Export
	detector     *Detector
	reporter     *Reporter
	catalog      *tables.Catalog
	price        *broker.Price
	desk         *broker.Desk
	storeTee     runtime.Tee
	uiTee        runtime.Tee
	epoch        int64
	detectorDone chan struct{}
	scanOnce     sync.Once
	episodes     *lf.OrderedMap[string, *episode]
	fragments    atomic.Pointer[[]ui.TrainedFragment]
	fragCount    atomic.Int64
	resolved     atomic.Int64
	wins         atomic.Int64
	returnsBits  atomic.Uint64
	passes       atomic.Int64
	skill        atomic.Int64
	baseline     atomic.Int64
	// gridRestore runs the grid/latest lookup once per process: a missing
	// checkpoint must not cost an object-storage round trip on every Step.
	gridRestore    sync.Once
	gridRestoreErr error
}

/*
episodeState holds one symbol's live state.
*/
type episodeState struct {
	window [][]byte
	entry  []byte
	exit   []byte
	mark   float64
	marked bool
}

/*
episode is one symbol's lock-free live state.
*/
type episode struct {
	state atomic.Pointer[episodeState]
}

func (episode *episode) setEntry(context []byte) {
	for {
		oldState := episode.state.Load()
		if oldState == nil {
			oldState = &episodeState{}
		}

		newState := *oldState
		newState.entry = context

		if episode.state.CompareAndSwap(oldState, &newState) {
			break
		}
	}
}

func (episode *episode) setExit(context []byte) {
	for {
		oldState := episode.state.Load()
		if oldState == nil {
			oldState = &episodeState{}
		}

		newState := *oldState
		newState.exit = context

		if episode.state.CompareAndSwap(oldState, &newState) {
			break
		}
	}
}

func (episode *episode) setMark(mark float64) {
	for {
		oldState := episode.state.Load()
		if oldState == nil {
			oldState = &episodeState{}
		}

		newState := *oldState
		newState.mark = mark
		newState.marked = true

		if episode.state.CompareAndSwap(oldState, &newState) {
			break
		}
	}
}

func (episode *episode) settle() ([]byte, []byte) {
	for {
		oldState := episode.state.Load()
		if oldState == nil {
			return nil, nil
		}

		newState := *oldState
		newState.entry = nil
		newState.exit = nil
		newState.marked = false

		if episode.state.CompareAndSwap(oldState, &newState) {
			return oldState.entry, oldState.exit
		}
	}
}

func (training *Training) addReturn(val float64) {
	for {
		oldBits := training.returnsBits.Load()
		newBits := math.Float64bits(math.Float64frombits(oldBits) + val)

		if training.returnsBits.CompareAndSwap(oldBits, newBits) {
			break
		}
	}
}

func (training *Training) getReturn() float64 {
	return math.Float64frombits(training.returnsBits.Load())
}

func NewTraining(
	ctx context.Context,
	arena *data.ArenaOwner,
	price *broker.Price,
	desk *broker.Desk,
	catalog *tables.Catalog,
	storeTee runtime.Tee,
	epoch int64,
) *Training {
	episodes := lf.NewOrderedMap[string, *episode](stringLess)
	emptyFragments := make([]ui.TrainedFragment, 0)
	memory := cognition.NewAssociate()

	training := &Training{
		System:       runtime.NewSystem(ctx, "training", price),
		arena:        arena,
		grid:         store.NewGrid(),
		memory:       memory,
		recall:       cognition.NewRecall(memory),
		trainer:      cognition.NewTrain(memory),
		census:       cognition.NewCensus(memory),
		snapshot:     cognition.NewSnapshot(memory),
		tree:         cognition.NewExport(memory),
		detector:     NewDetector(ctx, storeTee, price),
		reporter:     NewReporter(),
		catalog:      catalog,
		price:        price,
		desk:         desk,
		storeTee:     storeTee,
		epoch:        epoch,
		detectorDone: make(chan struct{}),
		episodes:     episodes,
	}

	training.fragments.Store(&emptyFragments)

	// The detector labels every excursion against friction. Without it the
	// training system halts here instead of learning from mislabeled tape.
	if err := training.detector.Error(); err != nil {
		training.Error(errnie.Err(errnie.Internal, "[training] detector is not usable", err))
		return training
	}

	desk.OnClose(training.settle)
	training.Transition(runtime.INIT)
	return training
}

func (training *Training) Arena() *data.ArenaOwner {
	return training.arena
}

func (training *Training) SetUITee(tee runtime.Tee) {
	if training == nil {
		return
	}

	training.uiTee = tee
}

func (training *Training) Fragments() []ui.TrainedFragment {
	if training == nil {
		return nil
	}

	ptr := training.fragments.Load()

	if ptr == nil {
		return nil
	}

	slice := *ptr
	out := make([]ui.TrainedFragment, len(slice))
	copy(out, slice)
	return out
}

func (training *Training) CognitionTree() ui.CognitionTreeExport {
	if training == nil || training.tree == nil {
		return ui.CognitionTreeExport{}
	}

	reading, err := training.ask(training.tree, nil, nil)

	if err != nil {
		errnie.Error(err)
		return ui.CognitionTreeExport{}
	}

	raw, ok, err := readText(reading, "tree")

	if err != nil {
		errnie.Error(err)
		return ui.CognitionTreeExport{}
	}

	if !ok {
		return ui.CognitionTreeExport{}
	}

	var export ui.CognitionTreeExport

	if err = json.Unmarshal([]byte(raw), &export); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition tree",
			err,
		))
		return ui.CognitionTreeExport{}
	}

	return export
}

func (training *Training) records() float64 {
	reading, err := training.ask(training.census, nil, nil)

	if err != nil {
		errnie.Error(err)
		return 0
	}

	value, ok, err := readNumber(reading, "records")

	if err != nil || !ok {
		return 0
	}

	return value
}

func (training *Training) classCount(name string) float64 {
	reading, err := training.ask(training.census, nil, nil)

	if err != nil {
		errnie.Error(err)
		return 0
	}

	value, ok, err := readNumber(reading, name)

	if err != nil || !ok {
		return 0
	}

	return value
}

func (training *Training) span() int {
	reading, err := training.ask(training.census, nil, nil)

	if err != nil {
		errnie.Error(err)
		return 0
	}

	value, ok, err := readNumber(reading, "span")

	if err != nil || !ok || value < 1 {
		return 0
	}

	return int(value)
}

func (training *Training) ask(
	primitive core.Primitive,
	text map[string]string,
	numbers map[string]float64,
) (*data.Adapter, error) {
	if training == nil || primitive == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition primitive is missing",
			nil,
		))
	}

	adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))

	if len(text) > 0 {
		issued := data.NewTextMap()

		for key, value := range text {
			issued.Values[key] = value
		}

		for range adapter.Next(data.NewValue(issued)) {
		}

		if err := adapter.Error(); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[training] cognition text",
				err,
			))
		}
	}

	if len(numbers) > 0 {
		issued := data.NewOutputMap()

		for key, value := range numbers {
			issued.Values[key] = value
		}

		for range adapter.Next(data.NewValue(issued)) {
		}

		if err := adapter.Error(); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[training] cognition numbers",
				err,
			))
		}
	}

	for range primitive.Next(data.NewValue(adapter)) {
	}

	if err := primitive.Error(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition",
			err,
		))
	}

	if err := adapter.Error(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition",
			err,
		))
	}

	return adapter, nil
}

func readNumber(adapter *data.Adapter, key string) (float64, bool, error) {
	if adapter == nil {
		return 0, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition adapter is missing",
			nil,
		))
	}

	var values data.Map[float64]

	for pointer := range adapter.Next(data.NewValue(data.NewMap(key, key))) {
		values = *(*data.Map[float64])(pointer)
	}

	if err := adapter.Error(); err != nil {
		return 0, false, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition number "+key,
			err,
		))
	}

	value, ok := values.Values[key]
	return value, ok, nil
}

func readText(adapter *data.Adapter, key string) (string, bool, error) {
	if adapter == nil {
		return "", false, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition adapter is missing",
			nil,
		))
	}

	var values data.Map[string]

	for pointer := range adapter.Next(data.NewValue(data.NewLiteral(key))) {
		values = *(*data.Map[string])(pointer)
	}

	if err := adapter.Error(); err != nil {
		return "", false, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] cognition text "+key,
			err,
		))
	}

	value, ok := values.Values[key]
	return value, ok, nil
}

/*
Step executes the sequential training, paper trading, and reporting steps for one measurement.
*/
func (training *Training) Step(prior *data.Measurement) *data.Measurement {
	if prior == nil {
		return nil
	}

	snapshot := ReportSnapshot{
		Source: training.Name(),
		Symbol: prior.Label,
		SeqIdx: prior.SeqIdx,
		At:     prior.At,
	}

	// Display only: sensory ticks that are not trades carry no price.
	// Absence leaves Price at 0 (omitted by the reporter); it never feeds
	// friction, profitability, or labels. A real read failure is logged —
	// never invent a price, and never treat it as absence.
	if price, err := readMetric(prior, "price"); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[training] display price read failed",
			err,
		))
	} else if price != nil {
		snapshot.Price = price.Raw
	}

	status := training.Status()
	sensory := sensoryMeasurements(prior)

	if len(sensory) > 0 {
		channels := channelsFrom(sensory...)

		if len(channels) > 0 {
			training.grid.Update(prior.Tick, channels)
		}
	}

	if status == runtime.INIT {
		training.develop(prior.Epoch, prior.SeqIdx, &snapshot)
	}

	if status == runtime.WAITING {
		snapshot.Stage = StageHistoricalValidation
		snapshot.Blocker = "loading trie from historical excursions"
	}

	if status == runtime.WAITING && training.passes.Load() > 0 {
		snapshot.Blocker = fmt.Sprintf(
			"skill gate: %d correct calls, constant-policy baseline %d",
			training.skill.Load(), training.baseline.Load(),
		)
	}

	if status == runtime.READY {
		training.trade(prior, &snapshot)
	}

	snapshot.Resolved, snapshot.WinRate, snapshot.Edge = training.score()

	peers := prior.Peers()

	if prior.Source != "runtime:join" {
		peers = []*data.Measurement{prior}
	}

	metadata := training.reporter.Metadata(snapshot)
	out := training.arena.NewMeasurement(
		prior.Epoch,
		prior.Label,
		training.Name(),
		prior.SeqIdx,
		prior.Tick,
		peers,
		metadata...,
	)
	out.At = prior.At
	out.From = prior.At

	training.reporter.Populate(out, snapshot)

	if training.uiTee != nil {
		training.uiTee.Push(data.NewPublication(out, nil))
	}

	return out
}

/*
develop grows the grid and checkpoints it once it settles.
A previously checkpointed grid (grid/latest) is restored first so a restart
does not re-discover regions that already froze.
*/
func (training *Training) develop(
	epoch, seqIdx int64, snapshot *ReportSnapshot,
) {
	snapshot.Stage = StageModelDevelopment
	snapshot.Blocker = "grid developing"

	if !training.grid.IsSettled() {
		restored, err := training.restoreGrid()

		if err != nil {
			// Internal closes training; root halts with this error.
			training.Error(errnie.Err(
				errnie.Internal,
				"[training] grid checkpoint restore failed",
				err,
			))

			return
		}

		if restored {
			snapshot.Blocker = "grid restored from checkpoint"
		} else {
			if seqIdx%32 == 0 {
				training.grid.Partition()
			}

			if !training.grid.Converged() {
				return
			}

			training.grid.Settle()
		}
	}

	if !training.checkpointGrid(epoch, seqIdx) {
		return
	}

	training.Transition(runtime.WAITING)
}

const gridLatestKey = "grid/latest"

/*
restoreGrid loads grid/latest when object storage is configured and a
checkpoint exists. Unconfigured storage or a missing checkpoint leaves the
live develop path (false, nil). A configured store that fails to read, or a
checkpoint that fails to restore, is an error: silently re-developing would
then overwrite grid/latest with a fresh grid and discard the trained one.
The lookup runs once per process; later calls only report IsSettled.
*/
func (training *Training) restoreGrid() (bool, error) {
	if training == nil || training.catalog == nil || training.grid == nil {
		return false, nil
	}

	if training.grid.IsSettled() {
		return true, nil
	}

	training.gridRestore.Do(func() {
		encoded, err := training.catalog.GetBlob(training.Context(), gridLatestKey)

		if errors.Is(err, tables.ErrBlobMissing) ||
			errors.Is(err, tables.ErrBlobStorageUnconfigured) {
			return
		}

		if err != nil {
			training.gridRestoreErr = errnie.Err(
				errnie.IO,
				"[training] unable to read grid/latest",
				err,
			)

			return
		}

		if err = training.grid.RestoreSnapshot(encoded); err != nil {
			training.gridRestoreErr = errnie.Err(
				errnie.IO,
				"[training] unable to restore grid/latest",
				err,
			)
		}
	})

	if training.gridRestoreErr != nil {
		return false, training.gridRestoreErr
	}

	return training.grid.IsSettled(), nil
}

/*
checkpointGrid snapshots the settled grid to grid/{epoch}/{seqIdx} and
grid/latest. Returns false when the snapshot itself cannot be taken.
*/
func (training *Training) checkpointGrid(epoch, seqIdx int64) bool {
	if training == nil || training.grid == nil {
		return false
	}

	encoded, err := training.grid.Snapshot()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[training] unable to snapshot grid/%d/%d", epoch, seqIdx),
			err,
		))
		return false
	}

	if training.catalog == nil {
		return true
	}

	keys := []string{
		fmt.Sprintf("grid/%d/%d", epoch, seqIdx),
		gridLatestKey,
	}

	go func(ctx context.Context, blob []byte, keys []string) {
		for _, key := range keys {
			if putErr := training.catalog.PutBlob(ctx, key, blob); putErr != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					fmt.Sprintf("[training] unable to checkpoint %s", key),
					putErr,
				))
			}
		}
	}(training.Context(), encoded, keys)

	return true
}

/*
trade classifies the symbol's rolling frame window and acts on the winner
when the Desk allows that action: enter when flat, exit when holding.
*/
func (training *Training) trade(prior *data.Measurement, snapshot *ReportSnapshot) {
	snapshot.Stage = StageForwardPaperLearning
	snapshot.Trading = true

	symbol := prior.Label
	sensory := sensoryMeasurements(prior)
	tokens := training.grid.LitRegions(channelsFrom(sensory...))
	snapshot.RegionTokens = tokens

	tok := training.token(sensory...)

	if len(tok) == 0 {
		return
	}

	span := training.span()
	current := training.episode(symbol)

	var question []byte
	for {
		oldState := current.state.Load()

		if oldState == nil {
			oldState = &episodeState{}
		}

		newState := *oldState

		if len(newState.window) == 0 || !bytes.Equal(newState.window[len(newState.window)-1], tok) {
			newWindow := make([][]byte, len(newState.window), len(newState.window)+1)
			copy(newWindow, newState.window)
			newWindow = append(newWindow, tok)
			limit := len(newWindow)

			if span >= 1 {
				limit = span
			}

			if len(newWindow) > limit {
				newWindow = newWindow[len(newWindow)-limit:]
			}

			newState.window = newWindow
		}

		question = bytes.Join(newState.window, []byte("/"))

		if current.state.CompareAndSwap(oldState, &newState) {
			break
		}
	}

	reading, err := training.ask(training.recall, map[string]string{
		"context": string(question),
	}, nil)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[training] unable to evaluate "+symbol, err))
		return
	}

	if confidence, ok, readErr := readNumber(reading, "confidence"); readErr == nil && ok {
		snapshot.Confidence = confidence
	}

	if contrast, ok, readErr := readNumber(reading, "contrast"); readErr == nil && ok {
		snapshot.Contrast = contrast
	}

	winner, _, err := readText(reading, "winner")

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[training] unable to evaluate "+symbol, err))
		return
	}

	state := training.desk.State(symbol)

	if state == broker.HOLDING {
		training.mark(symbol)
	}

	if winner == "" {
		return
	}

	if state == broker.FLAT && winner == actionEnter {
		resonanceM := solverMeasurement(prior, "resonance")
		manifoldM := solverMeasurement(prior, "manifold")

		if training.authorized(resonanceM, manifoldM) {
			training.act(
				symbol,
				training.desk.Enter,
				func(held *episode, context []byte) { held.setEntry(context) },
				question,
			)
			snapshot.Action = 1
		}
	}

	if state == broker.HOLDING && winner == actionExit {
		training.act(
			symbol,
			training.desk.Exit,
			func(held *episode, context []byte) { held.setExit(context) },
			question,
		)
		snapshot.Action = 2
	}
}

/*
channelsFrom extracts key-value telemetry pairs from measurements for grid updates.
*/
func channelsFrom(measurements ...*data.Measurement) map[string]float64 {
	channels := make(map[string]float64)

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		for entry := range measurement.Read() {
			if entry == nil || entry.Err != nil || entry.Metric == nil {
				continue
			}

			key := store.CellKey(measurement.Label, measurement.Source, entry.Metric.Label)
			channels[key] = entry.Metric.Raw
		}
	}

	return channels
}

/*
sensoryMeasurements filters a measurement and its peers to retain only Stage 0
sensory signal producers, excluding higher-order cognitive and physical solvers.
*/
func sensoryMeasurements(prior *data.Measurement) []*data.Measurement {
	if prior == nil {
		return nil
	}

	if prior.Source != "runtime:join" {
		if prior.Source != "resonance" && prior.Source != "manifold" {
			return []*data.Measurement{prior}
		}

		return nil
	}

	var sensory []*data.Measurement

	for _, peer := range prior.Peers() {
		if peer == nil {
			continue
		}

		if peer.Source == "resonance" || peer.Source == "manifold" {
			continue
		}

		sensory = append(sensory, peer)
	}

	return sensory
}

/*
solverMeasurement locates a specific solver producer measurement from prior or its peers.
*/
func solverMeasurement(prior *data.Measurement, source string) *data.Measurement {
	if prior == nil {
		return nil
	}

	if prior.Source == source {
		return prior
	}

	for _, peer := range prior.Peers() {
		if peer != nil && peer.Source == source {
			return peer
		}
	}

	return nil
}

/*
authorized applies the execution triad gate:
 1. Resonance: Verifies presence of structural shock / dislocation (Surprise > 0),
    vetoing entries during predictable equilibrium churn.
 2. Manifold: Verifies the order book medium permits wave propagation, vetoing
    entries if resting orders exhibit locked synchronization opposing the move.
*/
func (training *Training) authorized(
	resonanceM, manifoldM *data.Measurement,
) bool {
	if resonanceM != nil {
		surprise, err := readMetric(resonanceM, "surprise")

		if err == nil && surprise != nil && surprise.Raw <= 0 {
			return false
		}
	}

	if manifoldM == nil {
		return true
	}

	kuramoto, err := readMetric(manifoldM, "kuramoto_r")

	if err != nil || kuramoto == nil || kuramoto.Raw < 1.0 {
		return true
	}

	pressure, err := readMetric(manifoldM, "pressure_grad_norm")

	if err == nil && pressure != nil && pressure.Raw > 0 {
		return false
	}

	return true
}

/*
act records the context that triggers a Desk operation before submitting it,
because the resulting fill may settle before the operation returns. A
rejected operation withdraws the context.
*/
func (training *Training) act(
	symbol string, operation func(string) error, record func(*episode, []byte), question []byte,
) {
	record(training.episode(symbol), question)

	err := operation(symbol)

	if err == nil {
		return
	}

	errnie.Error(err)

	record(training.episode(symbol), nil)
}

/*
mark records the open position's unrealized return for the UI score.
Choppy marks never train the trie; only realized closures do.
*/
func (training *Training) mark(symbol string) {
	pnl, basis, err := training.desk.Unrealized(symbol)

	if err != nil {
		errnie.Error(err)
		return
	}

	held := training.episode(symbol)
	held.setMark(pnl.SetScale(decimal.DefaultScale).Div(basis).Float64())
}

/*
settle consumes one realized round trip from the Desk and refines the trie
on the contexts that entered and exited it. A winning entry reinforces enter
with its return. A losing entry is a losing precursor: it teaches wait with
the loss avoided, so live losses never strengthen enter. The exit context is
always taught exit: a winning round trip grades it with the return it
realized, a losing one with the loss magnitude, because closing a bad trade
was the right action and must not be weakened by the loss it stopped.
*/
func (training *Training) settle(closure broker.Closure) {
	feedback := closure.Realized.SetScale(decimal.DefaultScale).Div(closure.Cost).Float64()

	held := training.episode(closure.Symbol)
	entry, exit := held.settle()
	training.resolved.Add(1)
	training.addReturn(feedback)

	if feedback > 0 {
		training.wins.Add(1)
	}

	if entry == nil {
		errnie.Error(errnie.Err(
			errnie.Conflict,
			"[training] closed position has no entry context: "+closure.Symbol,
			nil,
		))

		return
	}

	entered, entryFeedback := actionEnter, feedback

	exitFeedback := feedback

	if feedback <= 0 {
		entered, entryFeedback = actionWait, -feedback
		exitFeedback = -feedback
	}

	if _, err := training.ask(training.trainer, map[string]string{
		"context": string(entry),
		"class":   entered,
	}, map[string]float64{
		"feedback": entryFeedback,
		"graded":   core.Unit,
	}); err != nil {
		errnie.Error(err)
	}

	if exit == nil {
		return
	}

	if _, err := training.ask(training.trainer, map[string]string{
		"context": string(exit),
		"class":   actionExit,
	}, map[string]float64{
		"feedback": exitFeedback,
		"graded":   core.Unit,
	}); err != nil {
		errnie.Error(err)
	}
}

/*
score reports win rate and edge over realized round trips plus the current
marks of open positions. It is display state, not training feedback.
*/
func (training *Training) score() (int64, float64, float64) {
	count := training.resolved.Load()
	wins := training.wins.Load()
	returns := training.getReturn()

	training.episodes.Range("", "\xff\xff\xff\xff", func(symbol string, held *episode) {
		if held == nil {
			return
		}

		state := held.state.Load()

		if state == nil || !state.marked {
			return
		}

		count++
		returns += state.mark

		if state.mark > 0 {
			wins++
		}
	})

	if count > 0 {
		return count, float64(wins) / float64(count), returns / float64(count)
	}

	if training.desk != nil && training.desk.Balance != nil {
		snap := training.desk.Balance.Snapshot()

		if snap != nil && snap.Unrealized != nil && snap.Equity != nil {
			unrealized := snap.Unrealized
			equity := snap.Equity
			base := equity.Sub(unrealized)

			if base.Sign() > 0 && unrealized.Sign() != 0 {
				ret := unrealized.SetScale(decimal.DefaultScale).Div(base).Float64()
				win := int64(0)

				if ret > 0 {
					win = 1
				}

				return 1, float64(win), ret
			}
		}
	}

	return 0, 0, 0
}

/*
episode returns the symbol's live state.
*/
func (training *Training) episode(symbol string) *episode {
	if held, ok := training.episodes.Get(symbol); ok && held != nil {
		return held
	}

	held := &episode{}
	held.state.Store(&episodeState{})
	training.episodes.Set(symbol, held)
	actual, _ := training.episodes.Get(symbol)
	return actual
}

/*
Train loads the trie once from the excursions of every past run, checkpoints
it, and opens paper trading. It waits for the grid to settle first, because
region tokens are only comparable once the grid is frozen. A past run without
stored detections has its trade tape scanned here, and its detections are
learned directly, so learning never waits on the asynchronous store. The
current run's tape is still being written and is never scanned.
*/
func (training *Training) Passes() int64 {
	return training.passes.Load()
}

func (training *Training) Train() {
	go func() {
		go training.runDetectorScan()

		for training.Status() == runtime.INIT {
			select {
			case <-training.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}

		select {
		case <-training.Context().Done():
			return
		case <-training.detectorDone:
		}

		if training.Status() != runtime.WAITING {
			return
		}

		var (
			trained   int
			latest    int64
			seenCount int
			hits      int
			asked     map[string]int
		)

		for rehearsal := 0; rehearsal < fragmentRehearsals; rehearsal++ {
			passTrained, passLatest, passSeen, passHits, passAsked, err := training.trainPass()

			// A failed pass halts training. Internal closes the training
			// context, which cmd/root watches, so the process stops with
			// this error instead of opening paper trading on a trie that
			// silently missed part of its tape. The error is recorded
			// before the pass is counted so a waiter never observes a
			// finished pass without its ERROR.
			if err != nil {
				training.Error(errnie.Err(errnie.Internal, "[training] failed during training pass", err))
				training.passes.Add(1)
				return
			}

			trained += passTrained
			hits = passHits
			asked = passAsked

			if passLatest > latest {
				latest = passLatest
			}

			if passSeen > seenCount {
				seenCount = passSeen
			}

			// Nothing to rehearse: stay WAITING without opening paper trading.
			if passTrained <= 0 && rehearsal == 0 {
				break
			}
		}

		records := training.records()
		calls, baseline := 0, 0

		for _, count := range asked {
			calls += count
			baseline = max(baseline, count)
		}

		training.skill.Store(int64(hits))
		training.baseline.Store(int64(baseline))

		/*
			Skill gate (TRAINING.md: paper trading starts once the model "has
			built up enough skill"). Every learned rehearsal asks the trie for
			the action at each of its phases before it is trained on them, so
			each call is graded against ground truth the trie has not yet seen
			at that A offset. A constant policy (always enter, always exit,
			always wait) scores exactly the calls whose truth is its action,
			and an abstaining trie scores zero. The best constant policy is
			therefore the largest single-action share of the latest pass's
			calls. Paper trading opens only when that pass beats it, and only
			once enter has ground truth to be graded on: a trie that has only
			learned wait and exit can never open a paper position.
		*/
		if trained == 0 || asked[actionEnter] == 0 || hits <= baseline {
			errnie.Info(fmt.Sprintf(
				"[training] skill gate closed: %d of %d calls correct, baseline %d (%d records)",
				hits, calls, baseline, int(records),
			))
			training.passes.Add(1)
			return
		}

		reading, askErr := training.ask(training.snapshot, nil, nil)

		// The snapshot reads the in-memory trie. Failing to read it means
		// the cognition memory itself is broken, so training halts instead
		// of opening paper trading on it.
		if askErr != nil {
			training.Error(errnie.Err(errnie.Internal, "[training] unable to snapshot trie", askErr))
			training.passes.Add(1)
			return
		}

		model, ok, readErr := readText(reading, "model")

		if readErr != nil {
			training.Error(errnie.Err(errnie.Internal, "[training] unable to snapshot trie", readErr))
			training.passes.Add(1)
			return
		}

		if !ok {
			training.Error(errnie.Err(
				errnie.Internal,
				"[training] cognition model is missing",
				nil,
			))
			training.passes.Add(1)
			return
		}

		putErr := training.catalog.PutBlob(
			training.Context(), fmt.Sprintf("trie/%d", latest), []byte(model),
		)

		// Object storage is optional (restoreGrid treats an unconfigured
		// bucket as "no checkpoint"), so a failed upload only loses the
		// restart checkpoint; the trie in memory is intact and correct.
		if putErr != nil {
			errnie.Error(errnie.Err(errnie.IO, "[training] unable to checkpoint trie", putErr))
		}

		errnie.Info(fmt.Sprintf(
			"[training] trie loaded from %d of %d excursions (%d records)",
			trained, seenCount, int(records),
		))

		training.Transition(runtime.READY)
		training.passes.Add(1)
	}()
}

/*
runDetectorScan runs as its own background process.
It finds the latest stored detection (source = detector), and scans any new
trade tape collected for epochs strictly before the current run's epoch
(epoch < training.epoch). A run that already has stored detections is never
scanned again: a classless detection left over from before the excursion
classes halts trainPass, so the fix is to reset those tables, not to append
classified rows next to it. Once all prior tape before the current epoch is
exhausted, the process exits.

A catalog read failure halts training with an Internal error, because the
runs it hides would look like runs that need no scan, or like an empty tape.
*/
func (training *Training) runDetectorScan() {
	defer training.scanOnce.Do(func() {
		close(training.detectorDone)
	})

	if training.catalog == nil {
		return
	}

	ctx := training.Context()

	var (
		latestEpoch int64
		latestTick  int64
	)

	for det, err := range training.catalog.Detections(ctx) {
		// Shutdown cancels the read; that is not a storage failure.
		if err != nil && ctx.Err() != nil {
			return
		}

		if err != nil {
			training.Error(errnie.Err(
				errnie.Internal, "[training] unable to read detections for detector scan", err,
			))
			return
		}

		if det == nil || det.Epoch >= training.epoch {
			continue
		}

		if det.Epoch > latestEpoch || (det.Epoch == latestEpoch && det.Tick > latestTick) {
			latestEpoch = det.Epoch
			latestTick = det.Tick
		}
	}

	// Without the run list no prior tape can be scanned, and training would
	// rehearse only what happens to be stored already.
	runs, err := training.catalog.Runs(ctx)
	if err != nil {
		training.Error(errnie.Err(errnie.Internal, "[training] unable to list runs for detector scan", err))
		return
	}

	slices.SortFunc(runs, func(left, right tables.Run) int {
		return cmp.Compare(left.Epoch, right.Epoch)
	})

	for _, run := range runs {
		if run.Epoch >= training.epoch {
			continue
		}

		if latestEpoch > 0 && run.Epoch < latestEpoch {
			continue
		}

		stored := false

		for det, err := range training.catalog.Detections(ctx, run.Epoch) {
			if err != nil && ctx.Err() != nil {
				return
			}

			if err != nil {
				training.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf("[training] unable to read detections for run %d", run.Epoch),
					err,
				))
				return
			}

			if det != nil {
				stored = true
				break
			}
		}

		if stored {
			continue
		}

		trades := training.catalog.Trades(ctx, run.Epoch)

		// A failed scan halts training: classifying the remaining tape
		// without friction would teach the trie mislabeled excursions, and
		// a tape whose read failed part-way would end its last excursion at
		// an arbitrary tick.
		if err := training.detector.Scan(trades); err != nil {
			training.Error(errnie.Err(errnie.Internal, "[training] detector scan failed", err))
			return
		}

		if training.storeTee != nil {
			writer := tables.NewWriter(training.catalog, run.Epoch)
			drained := 0

			for {
				ptr := training.storeTee.Next()
				if ptr == nil {
					break
				}

				pub := data.To[data.Publication](ptr)
				if pub.Measurement == nil {
					continue
				}

				writer.Add(tables.Measurements, pub)
				drained++
			}

			// Detections that fail to commit never reach trainPass, so the
			// trie would silently train without them.
			if drained > 0 {
				if commitErr := writer.CommitReady(ctx, true); commitErr != nil {
					training.Error(errnie.Err(
						errnie.Internal,
						fmt.Sprintf("[training] unable to commit detections for run %d", run.Epoch),
						commitErr,
					))
					return
				}
			}
		}
	}
}

/*
trainPass rehearses every stored excursion once and returns the fragments
learned, the latest epoch, the distinct excursions seen, the correct
prequential action calls made before learning them, and how many of those
calls each ground-truth action posed.
*/
func (training *Training) trainPass() (int, int64, int, int, map[string]int, error) {
	asked := make(map[string]int)

	if training.catalog == nil {
		return 0, 0, 0, 0, asked, nil
	}

	ctx := training.Context()
	seen := make(map[string]struct{})
	var (
		latest  int64
		trained int
		hits    int
	)

	for detection, err := range training.catalog.Detections(ctx) {
		// A failed read is not the end of the stored excursions: a pass
		// over the part that loaded would grade the trie on a subset.
		if ctx.Err() != nil {
			return 0, 0, 0, 0, nil, ctx.Err()
		}

		if err != nil {
			return 0, 0, 0, 0, nil, errnie.Err(
				errnie.IO, "[training] unable to read stored detections", err,
			)
		}

		if detection == nil || detection.Epoch >= training.epoch {
			continue
		}

		key := fmt.Sprintf(
			"%d/%s/%s/%d", detection.Epoch, detection.Label, detection.Meta("type"), detection.Tick,
		)
		if _, done := seen[key]; done {
			continue
		}

		seen[key] = struct{}{}
		if detection.Epoch > latest {
			latest = detection.Epoch
		}

		// A detection that cannot be learned (missing tape, inconsistent
		// class, unpriceable friction) stops the pass. Skipping it would
		// bias the trie and the skill gate toward the excursions that
		// happened to load.
		questions, correct, err := training.learn(detection)
		if err != nil {
			return 0, 0, 0, 0, nil, errnie.Err(
				errnie.Internal,
				"[training] unable to learn detection "+key,
				err,
			)
		}

		// Its tape exists, but its geometry forms no phase (see learnAt).
		if len(questions) == 0 {
			continue
		}

		trained++
		hits += correct

		for action, count := range questions {
			asked[action] += count
		}
	}

	return trained, latest, len(seen), hits, asked, nil
}

/*
learn cuts one stored excursion into its trainable phases. Every class maps
onto the three trie actions (enter, exit, wait) by what the right call was
at that point of the tape:

  - up: the precursor from a random A offset up to the frame before
    ignition B teaches enter, with the net round-trip return; the holding
    run from B+1 up to the frame before the peak C teaches exit, with the
    gross move captured.
  - up_friction: the precursor teaches wait, because entering loses after
    friction, with the loss avoided as feedback; the holding run still
    teaches exit, with the gross move captured.
  - down: the precursor before the top B teaches wait, with the loss
    avoided; the fall from B+1 to the frame before the bottom C teaches
    exit, with the fall avoided.
  - chop and flat: the stretch from a random A offset through C teaches
    wait, with the round-trip friction avoided.

Losing and quiet tape therefore never reinforces enter; it competes with
enter for the same contexts under wait.

A is offset randomly before B on every call (TRAINING.md fragment rehearsal)
so the trie does not only memorize one precursor length. ENTER/EXIT UI markers
sit on the sweet-spot frames of the phases that teach them, not on B/C.

Before any phase is trained, the trie is asked which action it would take on
it. The returned map counts the questions per ground-truth action, and the
returned count is how many of them the trie answered correctly; abstention is
a miss. An empty map means the excursion was not learned.
*/
func (training *Training) learn(detection *data.Measurement) (map[string]int, int, error) {
	return training.learnAt(detection, -1)
}

/*
learnAt is learn with an explicit A offset, counted in token frames from the
first frame A may occupy. Pass aOffset < 0 to draw a fresh random A.

Errors versus "nothing to learn": every stored detection must have its
signal/logic tape, and a detection whose tape is missing, or lights no grid
region, is data loss and returns an error that stops the pass. An empty map
with a nil error is returned only when the tape exists (or is never needed)
but the excursion's geometry cannot form a phase:

  - a directional excursion that ignites on the first tick of its tape has
    no precursor, so its tape is not read at all;
  - no token frame falls before ignition B (no precursor frame), or B and
    the peak C fall onto the same frame;
  - the holding run between B and C is too short to leave a frame once the
    fill-latency pullback before C is taken;
  - a chop/flat stretch has no token frame at or after B.
*/
func (training *Training) learnAt(detection *data.Measurement, aOffset int) (map[string]int, int, error) {
	if detection == nil {
		return nil, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] detection is required",
			nil,
		))
	}

	class := detection.Meta("type")

	startTick, bTick, cTick, err := tables.DetectionTicks(detection)
	if err != nil {
		return nil, 0, errnie.Error(err)
	}

	bPrice, cPrice, err := tables.DetectionPrices(detection)
	if err != nil {
		return nil, 0, errnie.Error(err)
	}

	net, err := training.net(detection.Label, bPrice, cPrice)
	if err != nil {
		return nil, 0, errnie.Error(err)
	}

	gross := cPrice.Sub(bPrice).SetScale(decimal.DefaultScale).Div(bPrice).Float64()

	// Legitimately empty: a directional excursion igniting on the first tick
	// of its tape has no precursor frame to learn, so its tape is not read.
	directional := class == excursionUp || class == excursionUpShort || class == excursionDown

	if directional && bTick <= startTick {
		return nil, 0, nil
	}

	// frames errors on a missing or region-less tape, so from here on the
	// tape exists and every "nothing to learn" return is geometry alone.
	ticks, tokens, err := training.frames(detection, startTick, cTick)
	if err != nil {
		return nil, 0, errnie.Error(err)
	}

	ignition, _ := slices.BinarySearch(ticks, bTick)
	peak, _ := slices.BinarySearch(ticks, cTick)

	drawA := func(from, to int) int {
		if to-from <= 1 {
			return from
		}

		if aOffset < 0 {
			return from + rand.IntN(to-from)
		}

		return from + min(aOffset, to-from-1)
	}

	var (
		phases []struct {
			context  string
			action   string
			feedback float64
		}
		startA     int
		enterFrame = -1
		exitFrame  = -1
	)

	// Every phase slice is non-empty and frames only keeps non-empty tokens,
	// so an empty context is a slicing bug, never a short window.
	phase := func(action string, feedback float64, frames [][]byte) bool {
		context := bytes.Join(deduplicateTokens(frames), []byte("/"))

		if len(context) == 0 {
			return false
		}

		phases = append(phases, struct {
			context  string
			action   string
			feedback float64
		}{string(context), action, feedback})

		return true
	}

	switch class {
	case excursionUp, excursionUpShort, excursionDown:
		// Legitimately empty: no frame before B, or B and C share a frame.
		if ignition < 1 || ignition >= peak {
			return nil, 0, nil
		}

		precursor := actionWait
		precursorFeedback := -net
		holdingFeedback := gross

		switch class {
		case excursionUp:
			if net <= 0 {
				return nil, 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"[training] up excursion does not clear friction: "+detection.Label,
					nil,
				))
			}

			precursor = actionEnter
			precursorFeedback = net
		case excursionUpShort:
			if net > 0 {
				return nil, 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"[training] up_friction excursion clears friction: "+detection.Label,
					nil,
				))
			}
		case excursionDown:
			if gross >= 0 {
				return nil, 0, errnie.Error(errnie.Err(
					errnie.Validation,
					"[training] down excursion does not fall: "+detection.Label,
					nil,
				))
			}

			holdingFeedback = -gross
		}

		// Pull back context before B and C to account for order fill latency.
		endB := ignition
		if ignition > 1 {
			endB = ignition - 1
		}

		startA = drawA(0, endB)

		startHolding := ignition + 1
		endC := peak
		if peak > startHolding+1 {
			endC = peak - 1
		}

		// Legitimately empty: the holding run is too short to leave a frame.
		if startHolding >= endC {
			return nil, 0, nil
		}

		if !phase(precursor, precursorFeedback, tokens[startA:endB]) ||
			!phase(actionExit, holdingFeedback, tokens[startHolding:endC]) {
			return nil, 0, errnie.Error(errnie.Err(
				errnie.Internal,
				"[training] "+class+" phase has an empty context: "+detection.Label,
				nil,
			))
		}

		// Sweet spots: last precursor frame before B, last holding frame before C.
		if precursor == actionEnter {
			enterFrame = max(endB-1, startA)
		}

		exitFrame = max(endC-1, startHolding)
	case excursionChop, excursionFlat:
		end := min(peak+1, len(tokens))

		// Legitimately empty: no frame at or after B inside the stretch.
		if ignition >= end {
			return nil, 0, nil
		}

		startA = drawA(ignition, end)

		if !phase(actionWait, -net, tokens[startA:end]) {
			return nil, 0, errnie.Error(errnie.Err(
				errnie.Internal,
				"[training] "+class+" phase has an empty context: "+detection.Label,
				nil,
			))
		}
	default:
		return nil, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] detection has unknown excursion class: \""+class+"\"",
			nil,
		))
	}

	for _, learned := range phases {
		if learned.feedback <= 0 {
			return nil, 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"[training] "+class+" "+learned.action+" phase has no positive feedback: "+detection.Label,
				nil,
			))
		}
	}

	asked := make(map[string]int, len(phases))
	hits := 0

	for _, learned := range phases {
		reading, err := training.ask(training.recall, map[string]string{
			"context": learned.context,
		}, nil)

		if err != nil {
			return nil, 0, errnie.Error(err)
		}

		winner, _, err := readText(reading, "winner")

		if err != nil {
			return nil, 0, errnie.Error(err)
		}

		asked[learned.action]++

		if winner == learned.action {
			hits++
		}
	}

	for _, learned := range phases {
		if _, err := training.ask(training.trainer, map[string]string{
			"context": learned.context,
			"class":   learned.action,
		}, map[string]float64{
			"feedback": learned.feedback,
			"graded":   core.Unit,
		}); err != nil {
			return nil, 0, errnie.Error(err)
		}
	}

	points, err := training.priceTape(detection, startTick, cTick)

	if err != nil {
		return nil, 0, err
	}

	tokenStrings := make([]string, len(tokens))

	for index, tok := range tokens {
		tokenStrings[index] = string(tok)
	}

	entryPointIdx := training.pointAt(points, ticks, enterFrame)
	exitPointIdx := training.pointAt(points, ticks, exitFrame)

	direction := "flat"

	if gross > 0 {
		direction = "up"
	}

	if gross < 0 {
		direction = "down"
	}

	fragment := ui.TrainedFragment{
		ID:         int(training.fragCount.Add(1)),
		Symbol:     detection.Label,
		Epoch:      detection.Epoch,
		MarkA:      ticks[startA],
		MarkB:      bTick,
		MarkC:      cTick,
		EntryPrice: bPrice.Float64(),
		ExitPrice:  cPrice.Float64(),
		Magnitude:  gross,
		Direction:  direction,
		Class:      class,
		Tokens:     tokenStrings,
		Points:     points,
		EntryIdx:   entryPointIdx,
		ExitIdx:    exitPointIdx,
		LearnedAt:  time.Now(),
	}

	for {
		oldPtr := training.fragments.Load()
		var next []ui.TrainedFragment

		if oldPtr != nil {
			next = make([]ui.TrainedFragment, len(*oldPtr)+1)
			copy(next, *oldPtr)
			next[len(*oldPtr)] = fragment
		}

		if oldPtr == nil {
			next = []ui.TrainedFragment{fragment}
		}

		if training.fragments.CompareAndSwap(oldPtr, &next) {
			break
		}
	}

	training.reporter.RecordFragment(class)
	training.streamFragment(detection, fragment)

	return asked, hits, nil
}

/*
pointAt maps a token frame index onto the fragment's price tape: the first
trade at or after the frame's tick, which is where an order placed on that
frame would fill. A negative frame, or a frame after the last trade, has no
marker and maps to -1. priceTape returns the points sorted by tick (Seq), the
key searched here, with X equal to each point's index.
*/
func (training *Training) pointAt(points []ui.FragmentPoint, ticks []int64, frame int) int {
	if frame < 0 || frame >= len(ticks) {
		return -1
	}

	index, _ := slices.BinarySearchFunc(points, ticks[frame], func(point ui.FragmentPoint, tick int64) int {
		return cmp.Compare(point.Tick, tick)
	})

	if index >= len(points) {
		return -1
	}

	return points[index].X
}

/*
net is the round-trip return of buying at entry and selling at exit after
taker friction, relative to the entry cost.
*/
func (training *Training) net(symbol string, entry, exit *decimal.Decimal) (float64, error) {
	if training.price == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] price system is required to grade excursions",
			nil,
		))
	}

	pnl, cost, err := training.price.RoundTrip(symbol, entry, exit)

	if err != nil {
		return 0, errnie.Error(err)
	}

	if pnl == nil || cost == nil || cost.Sign() <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] round trip has no cost basis: "+symbol,
			nil,
		))
	}

	return pnl.SetScale(decimal.DefaultScale).Div(cost).Float64(), nil
}

func deduplicateTokens(tokens [][]byte) [][]byte {
	var deduped [][]byte

	for _, tok := range tokens {
		if len(tok) == 0 {
			continue
		}

		if len(deduped) == 0 || !bytes.Equal(deduped[len(deduped)-1], tok) {
			deduped = append(deduped, tok)
		}
	}

	return deduped
}

/*
frames reads the excursion's signal and logic tape from the excursion start up to
the peak and encodes one region token per tick from all signal and logic steps.
A stored detection always has its tape, so a window with no signal/logic rows,
or whose rows light no grid region at all, is an error, never an empty window.
*/
func (training *Training) frames(
	detection *data.Measurement, startTick, highTick int64,
) ([]int64, [][]byte, error) {
	var (
		rawMeasurements []*data.Measurement
		ticks           []int64
		tokens          [][]byte
		group           []*data.Measurement
		current         int64 = -1
	)

	for measurement, err := range training.catalog.SignalLogic(
		training.Context(), detection.Epoch, detection.Label, startTick, highTick,
	) {
		// A failed read must not look like a missing or shorter tape.
		if err != nil {
			return nil, nil, errnie.Err(
				errnie.IO,
				fmt.Sprintf(
					"[training] unable to read signal/logic tape: %s epoch %d ticks %d..%d",
					detection.Label, detection.Epoch, startTick, highTick,
				),
				err,
			)
		}

		if measurement.Source == "resonance" || measurement.Source == "manifold" {
			continue
		}

		rawMeasurements = append(rawMeasurements, measurement)
	}

	if err := training.Context().Err(); err != nil {
		return nil, nil, err
	}

	if len(rawMeasurements) == 0 {
		return nil, nil, errnie.Err(
			errnie.NotFound,
			fmt.Sprintf(
				"[training] detection has no signal/logic tape: %s epoch %d ticks %d..%d",
				detection.Label, detection.Epoch, startTick, highTick,
			),
			nil,
		)
	}

	if !training.grid.IsSettled() {
		restored, err := training.restoreGrid()

		if err != nil {
			return nil, nil, err
		}

		if !restored {
			for _, measurement := range rawMeasurements {
				channels := channelsFrom(measurement)

				if len(channels) > 0 {
					training.grid.Update(measurement.Tick, channels)
				}
			}

			training.grid.Settle()
		}

		training.checkpointGrid(detection.Epoch, highTick)
	}

	flush := func() {
		if len(group) == 0 {
			return
		}

		tok := training.token(group...)
		group = group[:0]

		if len(tok) == 0 {
			return
		}

		ticks = append(ticks, current)
		tokens = append(tokens, tok)
	}

	for _, measurement := range rawMeasurements {
		if measurement.Tick != current {
			flush()
			current = measurement.Tick
		}

		group = append(group, measurement)
	}

	flush()

	if len(tokens) == 0 {
		return nil, nil, errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[training] signal/logic tape lights no grid region: %s epoch %d ticks %d..%d (%d rows)",
				detection.Label, detection.Epoch, startTick, highTick, len(rawMeasurements),
			),
			nil,
		)
	}

	return ticks, tokens, nil
}

func (training *Training) token(measurements ...*data.Measurement) []byte {
	lit := training.grid.LitRegions(channelsFrom(measurements...))

	if len(lit) == 0 {
		return nil
	}

	return bytes.Join(lit, []byte("_"))
}

/*
priceTape reads the fragment's chart points: the spot:trade rows of the
detection's label and epoch whose tick lies in [startTick, highTick], in tick
order (sequence index breaks ties), so X is the trade's position in tick order
and pointAt can binary-search the points by tick. FragmentPoint.Tick carries
the trade's tick.

Rows from any other source are ignored even when they carry a price metric,
and so are rows the Timeline admits only through its sequence-index bound.
Nothing is ever fabricated: a failed read, a trade without a positive exact
price, a trade without a timestamp, or a window holding no trade at all is an
error. The detector derives every excursion from this same trade tape, so an
empty window means storage and detections disagree.
*/
func (training *Training) priceTape(
	detection *data.Measurement,
	startTick, highTick int64,
) ([]ui.FragmentPoint, error) {
	if training.catalog == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[training] catalog is required to read the price tape",
			nil,
		))
	}

	window := fmt.Sprintf(
		"%s epoch %d ticks %d..%d", detection.Label, detection.Epoch, startTick, highTick,
	)

	var trades []*data.Measurement

	for measurement, err := range training.catalog.Timeline(
		training.Context(), detection.Epoch, detection.Label, startTick, highTick,
	) {
		if err != nil {
			return nil, errnie.Err(
				errnie.IO, "[training] unable to read price tape: "+window, err,
			)
		}

		if measurement == nil || measurement.Source != "spot:trade" {
			continue
		}

		// Timeline already bounds on tick; re-check so the fragment's
		// window never depends on the catalog's filtering.
		if measurement.Label != detection.Label ||
			measurement.Epoch != detection.Epoch ||
			measurement.Tick < startTick ||
			measurement.Tick > highTick {
			continue
		}

		trades = append(trades, measurement)
	}

	if len(trades) == 0 {
		return nil, errnie.Err(
			errnie.NotFound,
			"[training] detection has no spot:trade price tape: "+window,
			nil,
		)
	}

	// Timeline yields sequence-index order; pointAt searches by tick.
	slices.SortStableFunc(trades, func(left, right *data.Measurement) int {
		if order := cmp.Compare(left.Tick, right.Tick); order != 0 {
			return order
		}

		return cmp.Compare(left.SeqIdx, right.SeqIdx)
	})

	points := make([]ui.FragmentPoint, 0, len(trades))

	for _, trade := range trades {
		metric, err := readMetric(trade, "price")

		if err != nil || metric == nil || metric.Exact == nil || metric.Exact.Sign() <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[training] spot:trade without a positive exact price in price tape: %s (tick %d seq %d)",
					window, trade.Tick, trade.SeqIdx,
				),
				err,
			)
		}

		price := metric.Exact

		timeMs := trade.At.UnixMilli()

		if timeMs <= 0 {
			return nil, errnie.Err(
				errnie.Validation,
				fmt.Sprintf(
					"[training] spot:trade without a timestamp in price tape: %s (tick %d seq %d)",
					window, trade.Tick, trade.SeqIdx,
				),
				nil,
			)
		}

		points = append(points, ui.FragmentPoint{
			X:    len(points),
			Y:    price.Float64(),
			Tick: trade.Tick,
			Time: timeMs,
		})
	}

	return points, nil
}

func (training *Training) streamFragment(
	detection *data.Measurement,
	fragment ui.TrainedFragment,
) {
	if training == nil || training.uiTee == nil || detection == nil {
		return
	}

	var regionTokens [][]byte

	for _, tok := range fragment.Tokens {
		regionTokens = append(regionTokens, []byte(tok))
	}

	enters := fragment.EntryIdx >= 0 && fragment.EntryIdx < len(fragment.Points)
	action := 0

	if enters {
		action = 1
	}

	// The fragment is historical: its venue time is the detection's C, never
	// the wall clock at replay.
	if detection.At.IsZero() {
		training.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[training] detection without venue time (At): %s tick %d",
				detection.Label, detection.Tick,
			),
			nil,
		))
		return
	}

	snapshot := ReportSnapshot{
		Source:       training.Name(),
		Symbol:       detection.Label,
		SeqIdx:       fragment.MarkC,
		At:           detection.At,
		Stage:        StageHistoricalValidation,
		Blocker:      "historical validation",
		Action:       action,
		Confidence:   1.0,
		RegionTokens: regionTokens,
		MarkA:        fragment.MarkA,
		MarkB:        fragment.MarkB,
		MarkC:        fragment.MarkC,
		Price:        fragment.ExitPrice,
		ExcursionMag: fragment.Magnitude,
		Direction:    fragment.Class,
		Clears:       fragment.Class == excursionUp,
		Event:        "completed",
	}

	// ENTER/EXIT markers track the fill sweet spots stored on the fragment,
	// not ground-truth B/C. A fragment whose phases never teach that action
	// (wait-only chop/flat, the precursor of a losing excursion) has none.
	var markers []*data.Metric

	if enters {
		mark := float64(fragment.Points[fragment.EntryIdx].Tick)
		metric := data.NewMetric("agent_entry", mark, data.UnitCount, data.TimescaleInstantaneous)
		metric.Standardized = mark
		markers = append(markers, metric)
	}

	if fragment.ExitIdx >= 0 && fragment.ExitIdx < len(fragment.Points) {
		mark := float64(fragment.Points[fragment.ExitIdx].Tick)
		metric := data.NewMetric("agent_exit", mark, data.UnitCount, data.TimescaleInstantaneous)
		metric.Standardized = mark
		markers = append(markers, metric)
	}

	metadata := training.reporter.Metadata(snapshot)
	out := data.NewMeasurement(
		detection.Epoch,
		detection.Label,
		training.Name(),
		fragment.MarkC,
		fragment.MarkC,
		metadata...,
	)
	out.At = snapshot.At
	out.From = snapshot.At

	training.reporter.Populate(out, snapshot, markers...)
	training.uiTee.Push(data.NewPublication(out, nil))
}
