package strategy

import (
	"bytes"
	"cmp"
	"context"
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
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/ui"
)

var (
	_ ui.CognitionSource = (*Training)(nil)
	_ ui.FragmentsSource = (*Training)(nil)
)

/*
Training develops one grid, loads the trie from historical excursions, then
paper trades the live market with it. Realized round trips refine the trie.
*/
type Training struct {
	*runtime.System
	arena        *data.ArenaOwner
	grid         *store.Grid
	engine       *cognition.Engine
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
	ingress      chan *data.Measurement[float64]
	mu           sync.Mutex
	episodes     map[string]*episode
	fragments    []ui.TrainedFragment
	resolved     int64
	wins         int64
	returns      float64
	passes       atomic.Int64
}

/*
episode is one symbol's live state: the rolling frame window the engine is
asked about, the contexts that triggered the open position's entry and exit,
and the position's current unrealized return.
*/
type episode struct {
	window [][]byte
	entry  []byte
	exit   []byte
	mark   float64
	marked bool
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
	training := &Training{
		System:       runtime.NewSystem(ctx, "training", price),
		arena:        arena,
		grid:         store.NewGrid(),
		engine:       cognition.NewEngine(cognition.Config{MemoryScale: 1.0}),
		detector:     NewDetector(ctx, storeTee, price),
		reporter:     NewReporter(),
		catalog:      catalog,
		price:        price,
		desk:         desk,
		storeTee:     storeTee,
		epoch:        epoch,
		detectorDone: make(chan struct{}),
		ingress:      make(chan *data.Measurement[float64], 65536),
		episodes:     make(map[string]*episode),
	}

	desk.OnClose(training.settle)
	training.Transition(runtime.INIT)
	go training.offRampLoop()
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

	training.mu.Lock()
	defer training.mu.Unlock()

	out := make([]ui.TrainedFragment, len(training.fragments))
	copy(out, training.fragments)
	return out
}

func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	return training.engine.TreeExport()
}

/*
Step enqueues the live market measurement onto the internal off-ramp worker
channel and returns immediately, decoupling the Disruptor ring buffer from
training, classification, and reporting latency.
*/
func (training *Training) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if prior == nil {
		return nil
	}

	select {
	case <-training.Context().Done():
		return nil
	case training.ingress <- prior:
	default:
		errnie.Error(errnie.Err(
			errnie.Internal,
			"[training] ingress buffer saturated; dropping measurement to protect pipeline throughput",
			nil,
		))
	}

	return nil
}

/*
offRampLoop continuously drains the ingress channel sequentially in causal order,
executing grid updates, paper trading, and UI reporting.
*/
func (training *Training) offRampLoop() {
	for {
		select {
		case <-training.Context().Done():
			return
		case prior, ok := <-training.ingress:
			if !ok {
				return
			}

			training.process(prior)
		}
	}
}

/*
process executes the sequential training, paper trading, and reporting steps for one measurement.
*/
func (training *Training) process(prior *data.Measurement[float64]) {
	if prior == nil {
		return
	}

	out := data.NewMeasurement[float64](training.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
	out.SeqIdx = prior.SeqIdx
	out.Label = prior.Label
	out.At = prior.At
	out.Peers = prior.Peers

	if prior.Source != "runtime:join" {
		out.Peers = []*data.Measurement[float64]{prior}
	}

	snapshot := ReportSnapshot{
		Source: training.Name(),
		Symbol: prior.Label,
		SeqIdx: prior.SeqIdx,
		At:     prior.At,
		Price:  prior.GetMetric("price").Raw,
	}

	status := training.Status()
	sensory := sensoryMeasurements(prior)

	if len(sensory) > 0 {
		training.grid.Update(sensory...)
	}

	if status == runtime.INIT {
		training.develop(out, &snapshot)
	}

	if status == runtime.WAITING {
		snapshot.Stage = StageHistoricalValidation
		snapshot.Blocker = "loading trie from historical excursions"
	}

	if status == runtime.READY {
		training.trade(prior, &snapshot)
	}

	snapshot.Resolved, snapshot.WinRate, snapshot.Edge = training.score()
	training.reporter.Populate(out, snapshot)

	if training.uiTee != nil {
		training.uiTee.Push(data.NewPublication(out, nil))
	}
}

/*
develop grows the grid and checkpoints it once it settles.
*/
func (training *Training) develop(
	out *data.Measurement[float64], snapshot *ReportSnapshot,
) {
	snapshot.Stage = StageModelDevelopment
	snapshot.Blocker = "grid developing"

	if !training.grid.IsSettled() {
		return
	}

	training.grid.Settle()
	encoded, err := training.grid.Snapshot()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[training] unable to snapshot grid/%d/%d", out.Epoch, out.SeqIdx),
			err,
		))
	}

	if err == nil {
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
		}(training.Context(), out.Epoch, out.SeqIdx, encoded)
	}

	training.Transition(runtime.WAITING)
}

/*
trade classifies the symbol's rolling frame window and acts on the winner
when the Desk allows that action: enter when flat, exit when holding.
*/
func (training *Training) trade(prior *data.Measurement[float64], snapshot *ReportSnapshot) {
	snapshot.Stage = StageForwardPaperLearning
	snapshot.Trading = true

	symbol := prior.Label
	sensory := sensoryMeasurements(prior)
	tokens := training.grid.LitRegions(sensory...)
	snapshot.RegionTokens = tokens

	tok := training.token(sensory...)

	if len(tok) == 0 {
		return
	}

	training.mu.Lock()
	current := training.episode(symbol)

	if len(current.window) == 0 || !bytes.Equal(current.window[len(current.window)-1], tok) {
		current.window = append(current.window, tok)

		if overflow := len(current.window) - training.engine.Order(); overflow > 0 {
			current.window = current.window[overflow:]
		}
	}

	question := bytes.Join(current.window, []byte("/"))
	training.mu.Unlock()

	result, err := training.engine.Evaluate(question)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "[training] unable to evaluate "+symbol, err))
		return
	}

	snapshot.Confidence = result.Evaluation.Confidence
	snapshot.Contrast = result.Evaluation.Contrast

	action := cognition.Action(result.Evaluation.WinnerClass)
	state := training.desk.State(symbol)

	if state == broker.HOLDING {
		training.mark(symbol)
	}

	if state == broker.FLAT && action == cognition.ActionEnter {
		resonanceM := solverMeasurement(prior, "resonance")
		manifoldM := solverMeasurement(prior, "manifold")

		if training.authorized(resonanceM, manifoldM) {
			training.act(
				symbol,
				training.desk.Enter,
				func(held *episode, context []byte) { held.entry = context },
				question,
			)
			snapshot.Action = 1
		}
	}

	if state == broker.HOLDING && action == cognition.ActionExit {
		training.act(
			symbol,
			training.desk.Exit,
			func(held *episode, context []byte) { held.exit = context },
			question,
		)
		snapshot.Action = 2
	}
}

/*
sensoryMeasurements filters a measurement and its peers to retain only Stage 0
sensory signal producers, excluding higher-order cognitive and physical solvers.
*/
func sensoryMeasurements(prior *data.Measurement[float64]) []*data.Measurement[float64] {
	if prior == nil {
		return nil
	}

	if prior.Source != "runtime:join" {
		if prior.Source != "resonance" && prior.Source != "manifold" {
			return []*data.Measurement[float64]{prior}
		}

		return nil
	}

	var sensory []*data.Measurement[float64]

	for _, peer := range prior.Peers {
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
func solverMeasurement(prior *data.Measurement[float64], source string) *data.Measurement[float64] {
	if prior == nil {
		return nil
	}

	if prior.Source == source {
		return prior
	}

	for _, peer := range prior.Peers {
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
	resonanceM, manifoldM *data.Measurement[float64],
) bool {
	if resonanceM != nil {
		if surpriseMetric, ok := resonanceM.LookupMetric("surprise"); ok {
			if surpriseMetric.Raw <= 0 {
				return false
			}
		}
	}

	if manifoldM != nil {
		if rMetric, ok := manifoldM.LookupMetric("kuramoto_r"); ok {
			if rMetric.Raw >= 1.0 {
				if pressMetric, ok := manifoldM.LookupMetric("pressure_grad_norm"); ok && pressMetric.Raw > 0 {
					return false
				}
			}
		}
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
	training.mu.Lock()
	record(training.episode(symbol), question)
	training.mu.Unlock()

	err := operation(symbol)

	if err == nil {
		return
	}

	errnie.Error(err)

	training.mu.Lock()
	defer training.mu.Unlock()

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

	training.mu.Lock()
	defer training.mu.Unlock()

	held := training.episode(symbol)
	held.mark = pnl.SetScale(decimal.DefaultScale).Div(basis).Float64()
	held.marked = true
}

/*
settle consumes one realized round trip from the Desk and refines the trie
with its return on the contexts that entered and exited it.
*/
func (training *Training) settle(closure broker.Closure) {
	feedback := closure.Realized.SetScale(decimal.DefaultScale).Div(closure.Cost).Float64()

	training.mu.Lock()
	held := training.episode(closure.Symbol)
	entry, exit := held.entry, held.exit
	held.entry, held.exit, held.marked = nil, nil, false
	training.resolved++
	training.returns += feedback

	if feedback > 0 {
		training.wins++
	}

	training.mu.Unlock()

	if entry == nil {
		errnie.Error(errnie.Err(
			errnie.Conflict,
			"[training] closed position has no entry context: "+closure.Symbol,
			nil,
		))

		return
	}

	if _, err := training.engine.Train(
		entry, []byte(cognition.ActionEnter), feedback,
	); err != nil {
		errnie.Error(err)
	}

	if exit == nil {
		return
	}

	if _, err := training.engine.Train(
		exit, []byte(cognition.ActionExit), feedback,
	); err != nil {
		errnie.Error(err)
	}
}

/*
score reports win rate and edge over realized round trips plus the current
marks of open positions. It is display state, not training feedback.
*/
func (training *Training) score() (int64, float64, float64) {
	training.mu.Lock()
	defer training.mu.Unlock()

	count, wins, returns := training.resolved, training.wins, training.returns

	for _, held := range training.episodes {
		if !held.marked {
			continue
		}

		count++
		returns += held.mark

		if held.mark > 0 {
			wins++
		}
	}

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
episode returns the symbol's live state. The caller holds training.mu.
*/
func (training *Training) episode(symbol string) *episode {
	held, ok := training.episodes[symbol]

	if !ok {
		held = &episode{}
		training.episodes[symbol] = held
	}

	return held
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
		training.passes.Add(1)

		if err != nil {
			training.Error(errnie.Err(errnie.BadGateway, "[training] failed during training pass", err))
			return
		}

		if trained > 0 && training.engine.Len() > 0 {
			snapshot, err := training.engine.Snapshot()

			if err == nil {
				err = training.catalog.PutBlob(
					training.Context(), fmt.Sprintf("trie/%d", latest), snapshot.Model,
				)
			}

			if err != nil {
				errnie.Error(errnie.Err(errnie.IO, "[training] unable to checkpoint trie", err))
			}

			errnie.Info(fmt.Sprintf(
				"[training] trie loaded from %d of %d excursions (%d records)",
				trained, seenCount, training.engine.Len(),
			))

			training.Transition(runtime.READY)
		}
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
func (training *Training) learn(detection *data.Measurement[float64]) (bool, error) {
	if detection == nil {
		return false, nil
	}

	lowTick, highTick, err := tables.DetectionTicks(detection)
	if err != nil {
		return false, errnie.Error(err)
	}

	startTick := int64(0)
	if metric, ok := detection.LookupMetric("StartTick"); ok {
		startTick = int64(metric.Raw)
	}

	if startTick == 0 {
		if metric, ok := detection.LookupMetric("start_tick"); ok {
			startTick = int64(metric.Raw)
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

	if ignition < 1 || ignition >= peak {
		return false, nil
	}

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

	if len(enter) == 0 || len(hold) == 0 {
		return false, nil
	}

	if _, err := training.engine.Train(
		enter, []byte(cognition.ActionEnter), feedback,
	); err != nil {
		return false, errnie.Error(err)
	}

	if _, err := training.engine.Train(
		hold, []byte(cognition.ActionExit), feedback,
	); err != nil {
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

	fragment := ui.TrainedFragment{
		ID:         len(training.fragments) + 1,
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

	training.mu.Lock()
	training.fragments = append(training.fragments, fragment)
	training.mu.Unlock()

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
	detection *data.Measurement[float64], startTick, highTick int64,
) ([]int64, [][]byte, error) {
	if !training.grid.IsSettled() {
		for measurement := range training.catalog.SignalLogic(
			training.Context(), detection.Epoch, detection.Label, startTick, highTick,
		) {
			if measurement.Source == "resonance" || measurement.Source == "manifold" {
				continue
			}

			training.grid.Update(measurement)
		}

		training.grid.Settle()
	}

	var (
		ticks   []int64
		tokens  [][]byte
		group   []*data.Measurement[float64]
		current int64 = -1
	)

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

	for measurement := range training.catalog.SignalLogic(
		training.Context(), detection.Epoch, detection.Label, startTick, highTick,
	) {
		if measurement.Tick != current {
			flush()
			current = measurement.Tick
		}

		if measurement.Source == "resonance" || measurement.Source == "manifold" {
			continue
		}

		group = append(group, measurement)
	}

	flush()
	return ticks, tokens, nil
}

func (training *Training) token(measurements ...*data.Measurement[float64]) []byte {
	lit := training.grid.LitRegions(measurements...)

	if len(lit) == 0 {
		return nil
	}

	return bytes.Join(lit, []byte("_"))
}

func (training *Training) priceTape(
	detection *data.Measurement[float64],
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

			priceMetric := measurement.GetMetric("price")

			if priceMetric.Raw <= 0 {
				continue
			}

			timeMs := measurement.At.UnixMilli()

			if timeMs <= 0 {
				timeMs = time.Now().UnixMilli()
			}

			points = append(points, ui.FragmentPoint{
				X:    len(points),
				Y:    priceMetric.Raw,
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
	detection *data.Measurement[float64],
	fragment ui.TrainedFragment,
) {
	if training == nil || training.uiTee == nil {
		return
	}

	out := data.NewMeasurement[float64](training.Name())
	out.Label = detection.Label
	out.SeqIdx = fragment.MarkC
	out.Tick = fragment.MarkC

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

	training.reporter.Populate(out, snapshot)

	out.SetMetric("agent_entry", data.NewMetric[float64](
		"agent_entry",
		data.UnitCount,
		data.TimescaleInstantaneous,
		0,
		1,
	).Write(float64(fragment.MarkB)))

	out.SetMetric("agent_exit", data.NewMetric[float64](
		"agent_exit",
		data.UnitCount,
		data.TimescaleInstantaneous,
		0,
		1,
	).Write(float64(fragment.MarkC)))

	training.uiTee.Push(data.NewPublication(out, nil))
}
