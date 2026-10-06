package strategy

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
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
		Price:  data.Pull(prior.Read("price")).Metric.Raw,
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
*/
func (training *Training) develop(
	epoch, seqIdx int64, snapshot *ReportSnapshot,
) {
	snapshot.Stage = StageModelDevelopment
	snapshot.Blocker = "grid developing"

	if !training.grid.IsSettled() {
		if seqIdx%32 == 0 {
			training.grid.Partition()
		}

		if !training.grid.Converged() {
			return
		}

		training.grid.Settle()
	}

	encoded, err := training.grid.Snapshot()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[training] unable to snapshot grid/%d/%d", epoch, seqIdx),
			err,
		))

		return
	}

	go func(ctx context.Context, epoch, seqIdx int64, blob []byte) {
		if putErr := training.catalog.PutBlob(
			ctx,
			fmt.Sprintf("grid/%d/%d", epoch, seqIdx),
			blob,
		); putErr != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				fmt.Sprintf("[training] unable to checkpoint grid/%d/%d", epoch, seqIdx),
				putErr,
			))
		}
	}(training.Context(), epoch, seqIdx, encoded)

	training.Transition(runtime.WAITING)
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
			if entry.Err != nil {
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
		surpriseEntry := data.Pull(resonanceM.Read("surprise"))

		if surpriseEntry.Err == nil && surpriseEntry.Metric.Label == "surprise" && surpriseEntry.Metric.Raw <= 0 {
			return false
		}
	}

	if manifoldM == nil {
		return true
	}

	kuramotoEntry := data.Pull(manifoldM.Read("kuramoto_r"))

	if kuramotoEntry.Err != nil || kuramotoEntry.Metric.Label != "kuramoto_r" || kuramotoEntry.Metric.Raw < 1.0 {
		return true
	}

	pressureEntry := data.Pull(manifoldM.Read("pressure_grad_norm"))

	if pressureEntry.Err == nil && pressureEntry.Metric.Label == "pressure_grad_norm" && pressureEntry.Metric.Raw > 0 {
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
with its return on the contexts that entered and exited it.
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

	if _, err := training.ask(training.trainer, map[string]string{
		"context": string(entry),
		"class":   actionEnter,
	}, map[string]float64{
		"feedback": feedback,
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
		"feedback": feedback,
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

		trained, latest, seenCount, err := training.trainPass()

		if err != nil {
			training.passes.Add(1)
			training.Error(errnie.Err(errnie.BadGateway, "[training] failed during training pass", err))
			return
		}

		records := training.records()

		if trained <= 0 || records <= 0 {
			training.passes.Add(1)
			return
		}

		reading, askErr := training.ask(training.snapshot, nil, nil)

		if askErr != nil {
			errnie.Error(errnie.Err(errnie.IO, "[training] unable to checkpoint trie", askErr))
			training.Transition(runtime.READY)
			training.passes.Add(1)
			return
		}

		model, ok, readErr := readText(reading, "model")

		if readErr != nil {
			errnie.Error(errnie.Err(errnie.IO, "[training] unable to checkpoint trie", readErr))
			training.Transition(runtime.READY)
			training.passes.Add(1)
			return
		}

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[training] cognition model is missing",
				nil,
			))
			training.Transition(runtime.READY)
			training.passes.Add(1)
			return
		}

		putErr := training.catalog.PutBlob(
			training.Context(), fmt.Sprintf("trie/%d", latest), []byte(model),
		)

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
It finds the latest measurement where source = detector, and scans any new
trade tape collected for epochs strictly before the current run's epoch
(epoch < training.epoch). Once all prior tape before the current epoch is
exhausted, the process exits.
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

	for det := range training.catalog.Detections(ctx) {
		if det == nil || det.Epoch >= training.epoch {
			continue
		}

		if det.Epoch > latestEpoch || (det.Epoch == latestEpoch && det.Tick > latestTick) {
			latestEpoch = det.Epoch
			latestTick = det.Tick
		}
	}

	runs, err := training.catalog.Runs(ctx)
	if err != nil {
		errnie.Error(err)
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

		existingCount := 0
		for range training.catalog.Detections(ctx, run.Epoch) {
			existingCount++
			break
		}

		if existingCount > 0 {
			continue
		}

		trades := training.catalog.Trades(ctx, run.Epoch)
		training.detector.Scan(trades)

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

			if drained > 0 {
				if commitErr := writer.CommitReady(ctx, true); commitErr != nil {
					errnie.Error(commitErr)
				}
			}
		}
	}
}

func (training *Training) trainPass() (int, int64, int, error) {
	if training.catalog == nil {
		return 0, 0, 0, nil
	}

	ctx := training.Context()
	seen := make(map[string]struct{})
	var (
		latest  int64
		trained int
	)

	for detection := range training.catalog.Detections(ctx) {
		if ctx.Err() != nil {
			return 0, 0, 0, ctx.Err()
		}

		if detection == nil || detection.Epoch >= training.epoch {
			continue
		}

		key := fmt.Sprintf("%d/%s/%d", detection.Epoch, detection.Label, detection.Tick)
		if _, done := seen[key]; done {
			continue
		}

		seen[key] = struct{}{}
		if detection.Epoch > latest {
			latest = detection.Epoch
		}

		learned, err := training.learn(detection)
		if err != nil {
			errnie.Error(err)
			continue
		}

		if learned {
			trained++
		}
	}

	return trained, latest, len(seen), nil
}

/*
learn cuts one excursion into its two trainable pieces and trains each with
the return multiplier from entry to exit:
  - enter: precursor frames up to the frame before ignition B, leaving
    the ignition tick itself for the market fill;
  - exit: frames from ignition B+1 up to the frame before the peak C, leaving
    the peak tick for the exit fill.
*/
func (training *Training) learn(detection *data.Measurement) (bool, error) {
	if detection == nil {
		return false, nil
	}

	lowTick, highTick, err := tables.DetectionTicks(detection)
	if err != nil {
		return false, errnie.Error(err)
	}

	startTick := int64(0)
	startEntry := data.Pull(detection.Read("StartTick"))

	if startEntry.Err == nil && startEntry.Metric.Label == "StartTick" {
		startTick = int64(startEntry.Metric.Raw)
	}

	if startTick == 0 {
		lowerEntry := data.Pull(detection.Read("start_tick"))

		if lowerEntry.Err == nil && lowerEntry.Metric.Label == "start_tick" {
			startTick = int64(lowerEntry.Metric.Raw)
		}
	}

	entry, exit, err := tables.DetectionPrices(detection)
	if err != nil {
		return false, errnie.Error(err)
	}

	feedback := exit.Sub(entry).SetScale(decimal.DefaultScale).Div(entry).Float64()
	ticks, tokens, err := training.frames(detection, startTick, highTick)
	if err != nil {
		return false, errnie.Error(err)
	}

	ignition, _ := slices.BinarySearch(ticks, lowTick)
	peak, _ := slices.BinarySearch(ticks, highTick)

	// Pull back context before ignition and peak to account for order fill latencies.
	startA := 0
	endB := ignition
	if ignition > 1 {
		endB = ignition - 1
	}

	precursorTokens := deduplicateTokens(tokens[startA:endB])
	enter := bytes.Join(precursorTokens, []byte("/"))

	startHolding := ignition + 1
	endC := peak
	if peak > startHolding+1 {
		endC = peak - 1
	}

	holdingTokens := deduplicateTokens(tokens[startHolding:endC])
	hold := bytes.Join(holdingTokens, []byte("/"))

	if ignition < 1 || ignition >= peak {
		return false, nil
	}

	if len(enter) == 0 || len(hold) == 0 {
		return false, nil
	}

	if _, err := training.ask(training.trainer, map[string]string{
		"context": string(enter),
		"class":   actionEnter,
	}, map[string]float64{
		"feedback": feedback,
		"graded":   core.Unit,
	}); err != nil {
		return false, errnie.Error(err)
	}

	if _, err := training.ask(training.trainer, map[string]string{
		"context": string(hold),
		"class":   actionExit,
	}, map[string]float64{
		"feedback": feedback,
		"graded":   core.Unit,
	}); err != nil {
		return false, errnie.Error(err)
	}

	points := training.priceTape(detection, startTick, highTick, entry, exit, ticks)
	tokenStrings := make([]string, len(tokens))

	for index, tok := range tokens {
		tokenStrings[index] = string(tok)
	}

	entryPointIdx := ignition
	exitPointIdx := peak

	for _, pt := range points {
		if pt.Seq == lowTick {
			entryPointIdx = pt.X
		}

		if pt.Seq == highTick {
			exitPointIdx = pt.X
		}
	}

	if len(points) > 0 {
		if entryPointIdx >= len(points) {
			entryPointIdx = len(points) - 1
		}

		if exitPointIdx <= entryPointIdx {
			exitPointIdx = min(entryPointIdx+1, len(points)-1)
		}
	}

	fragID := int(training.fragCount.Add(1))

	fragment := ui.TrainedFragment{
		ID:         fragID,
		Symbol:     detection.Label,
		Epoch:      detection.Epoch,
		MarkA:      startTick,
		MarkB:      lowTick,
		MarkC:      highTick,
		EntryPrice: entry.Float64(),
		ExitPrice:  exit.Float64(),
		Magnitude:  feedback,
		Direction:  "up",
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

	training.streamFragment(detection, fragment)

	return true, nil
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

	for measurement := range training.catalog.SignalLogic(
		training.Context(), detection.Epoch, detection.Label, startTick, highTick,
	) {
		if measurement.Source == "resonance" || measurement.Source == "manifold" {
			continue
		}

		rawMeasurements = append(rawMeasurements, measurement)
	}

	if len(rawMeasurements) == 0 {
		return nil, nil, nil
	}

	if !training.grid.IsSettled() {
		for _, measurement := range rawMeasurements {
			channels := channelsFrom(measurement)

			if len(channels) > 0 {
				training.grid.Update(measurement.Tick, channels)
			}
		}

		training.grid.Settle()
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
	return ticks, tokens, nil
}

func (training *Training) token(measurements ...*data.Measurement) []byte {
	lit := training.grid.LitRegions(channelsFrom(measurements...))

	if len(lit) == 0 {
		return nil
	}

	return bytes.Join(lit, []byte("_"))
}

func (training *Training) priceTape(
	detection *data.Measurement,
	startTick, highTick int64,
	entry, exit *decimal.Decimal,
	ticks []int64,
) []ui.FragmentPoint {
	var points []ui.FragmentPoint

	if training.catalog != nil {
		for measurement := range training.catalog.Timeline(
			training.Context(), detection.Epoch, detection.Label, startTick, highTick,
		) {
			if measurement == nil {
				continue
			}

			entry := data.Pull(measurement.Read("price"))

			if entry.Err != nil || entry.Metric.Raw <= 0 {
				continue
			}

			timeMs := measurement.At.UnixMilli()

			if timeMs <= 0 {
				timeMs = time.Now().UnixMilli()
			}

			points = append(points, ui.FragmentPoint{
				X:    len(points),
				Y:    entry.Metric.Raw,
				Seq:  measurement.Tick,
				Time: timeMs,
			})
		}
	}

	if len(points) == 0 && len(ticks) > 0 {
		entryVal := entry.Float64()
		exitVal := exit.Float64()
		span := float64(len(ticks))

		for idx, tick := range ticks {
			frac := float64(idx) / math.Max(span-1, 1.0)
			val := entryVal + (exitVal-entryVal)*frac

			points = append(points, ui.FragmentPoint{
				X:    idx,
				Y:    val,
				Seq:  tick,
				Time: time.Now().UnixMilli(),
			})
		}
	}

	return points
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

	snapshot := ReportSnapshot{
		Source:       training.Name(),
		Symbol:       detection.Label,
		SeqIdx:       fragment.MarkC,
		At:           time.Now(),
		Stage:        StageHistoricalValidation,
		Blocker:      "historical validation",
		Action:       1,
		Confidence:   1.0,
		RegionTokens: regionTokens,
		MarkA:        fragment.MarkA,
		MarkB:        fragment.MarkB,
		MarkC:        fragment.MarkC,
		Price:        fragment.ExitPrice,
		ExcursionMag: fragment.Magnitude,
		Direction:    fragment.Direction,
		Clears:       true,
		Event:        "completed",
	}

	entryMetric := data.NewMetric("agent_entry", float64(fragment.MarkB), data.UnitCount, data.TimescaleInstantaneous)
	entryMetric.Standardized = float64(fragment.MarkB)

	exitMetric := data.NewMetric("agent_exit", float64(fragment.MarkC), data.UnitCount, data.TimescaleInstantaneous)
	exitMetric.Standardized = float64(fragment.MarkC)

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

	training.reporter.Populate(out, snapshot, entryMetric, exitMetric)
	training.uiTee.Push(data.NewPublication(out, nil))
}
