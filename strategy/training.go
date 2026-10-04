package strategy

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/ui"
)

var _ ui.CognitionSource = (*Training)(nil)

/*
Training develops one grid, loads the trie from historical excursions, then
paper trades the live market with it. Realized round trips refine the trie.
*/
type Training struct {
	*runtime.System
	arena    *data.ArenaOwner
	grid     *store.Grid
	engine   *cognition.Engine
	detector *Detector
	reporter *Reporter
	catalog  *tables.Catalog
	price    *broker.Price
	desk     *broker.Desk
	mu       sync.Mutex
	episodes map[string]*episode
	resolved int64
	wins     int64
	returns  float64
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
) *Training {
	training := &Training{
		System:   runtime.NewSystem(ctx, "training", price),
		arena:    arena,
		grid:     store.NewGrid(),
		engine:   cognition.NewEngine(cognition.Config{}),
		detector: NewDetector(ctx, storeTee),
		reporter: NewReporter(),
		catalog:  catalog,
		price:    price,
		desk:     desk,
		episodes: make(map[string]*episode),
	}

	desk.OnClose(training.settle)
	training.Transition(runtime.INIT)
	return training
}

func (training *Training) Arena() *data.ArenaOwner {
	return training.arena
}

func (training *Training) CognitionTree() cognition.CognitionTreeExport {
	return training.engine.TreeExport()
}

/*
Step processes the live market signal across three operational stages:
 1. Grid development (INIT): develops the grid until settled, then checkpoints it.
 2. Trie loading (WAITING): Train loads historical excursions into the trie.
 3. Paper trading (READY): live frames are classified and traded through the Desk.
*/
func (training *Training) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if prior == nil {
		return nil
	}

	out := training.arena.NewMeasurement(training.Name())
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

	if status == runtime.INIT {
		training.develop(prior, out, &snapshot)
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
	return out
}

/*
develop grows the grid and checkpoints it once it settles.
*/
func (training *Training) develop(
	prior, out *data.Measurement[float64], snapshot *ReportSnapshot,
) {
	snapshot.Stage = StageModelDevelopment
	snapshot.Blocker = "grid developing"
	training.grid.Update(prior)

	if !training.grid.IsSettled() {
		return
	}

	training.grid.Settle()
	encoded, err := training.grid.Snapshot()

	if err == nil {
		err = training.catalog.PutBlob(
			training.Context(),
			fmt.Sprintf("grid/%d/%d", out.Epoch, out.SeqIdx),
			encoded,
		)
	}

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("[training] unable to checkpoint grid/%d/%d", out.Epoch, out.SeqIdx),
			err,
		))
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
	tokens := training.grid.LitRegions(prior)
	snapshot.RegionTokens = tokens

	tok := training.token(prior)

	if len(tok) == 0 {
		return
	}

	training.mu.Lock()
	current := training.episode(symbol)
	current.window = append(current.window, tok)

	if overflow := len(current.window) - training.engine.Order(); overflow > 0 {
		current.window = current.window[overflow:]
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
		training.act(
			symbol,
			training.desk.Enter,
			func(held *episode, context []byte) { held.entry = context },
			question,
		)
		snapshot.Action = 1
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
	held.mark = pnl.Div(basis).Float64()
	held.marked = true
}

/*
settle consumes one realized round trip from the Desk and refines the trie
with its return on the contexts that entered and exited it.
*/
func (training *Training) settle(closure broker.Closure) {
	feedback := closure.Realized.Div(closure.Cost).Float64()

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

	if count == 0 {
		return 0, 0, 0
	}

	return count, float64(wins) / float64(count), returns / float64(count)
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
Train loads the trie once from every stored excursion, checkpoints it, and
opens paper trading. It waits for the grid to settle first, because region
tokens are only comparable once the grid is frozen.
*/
func (training *Training) Train() {
	go func() {
		for training.Status() == runtime.INIT {
			select {
			case <-training.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}

		runs, err := training.catalog.Runs(training.Context())

		if err != nil {
			training.Error(errnie.Err(errnie.BadGateway, "[training] failed to query runs", err))
			return
		}

		slices.SortFunc(runs, func(left, right tables.Run) int {
			return cmp.Compare(left.Epoch, right.Epoch)
		})

		seen := make(map[string]struct{})
		var latest int64

		for _, run := range runs {
			for detection := range training.catalog.Detections(training.Context(), run.Epoch) {
				if training.Context().Err() != nil {
					return
				}

				key := fmt.Sprintf("%d/%s/%d", detection.Epoch, detection.Label, detection.Tick)

				if _, done := seen[key]; done {
					continue
				}

				seen[key] = struct{}{}
				latest = run.Epoch

				if err := training.learn(detection); err != nil {
					errnie.Error(err)
				}
			}
		}

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
			"[training] trie loaded from %d excursions (%d records)", len(seen), training.engine.Len(),
		))

		training.Transition(runtime.READY)
	}()
}

/*
learn cuts one excursion into its two trainable pieces and trains each with
the round trip's return as priced by Price:
  - enter: a random start A up to the last frame before ignition B, leaving
    the ignition tick itself for the market fill;
  - exit: one frame after B up to the last frame before the peak C, leaving
    the peak tick for the exit fill.
*/
func (training *Training) learn(detection *data.Measurement[float64]) error {
	lowTick, highTick, err := tables.DetectionTicks(detection)

	if err != nil {
		return errnie.Error(err)
	}

	startTick := int64(0)

	if metric, ok := detection.LookupMetric("StartTick"); ok {
		startTick = int64(metric.Raw)
	} else if metric, ok := detection.LookupMetric("start_tick"); ok {
		startTick = int64(metric.Raw)
	}

	entry, exit, err := tables.DetectionPrices(detection)

	if err != nil {
		return errnie.Error(err)
	}

	pnl, total, err := training.price.RoundTrip(detection.Label, entry, exit)

	if err != nil {
		return errnie.Error(err)
	}

	feedback := pnl.Div(total).Float64()
	ticks, tokens, err := training.frames(detection, startTick, highTick)

	if err != nil {
		return errnie.Error(err)
	}

	ignition, _ := slices.BinarySearch(ticks, lowTick)
	peak, _ := slices.BinarySearch(ticks, highTick)

	if ignition < 1 || ignition+1 >= peak {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf(
				"[training] excursion %d/%s lacks frames around ignition",
				detection.Epoch, detection.Label,
			),
			nil,
		))
	}

	start := rand.IntN(ignition)
	enter := bytes.Join(tokens[start:ignition], []byte("/"))
	hold := bytes.Join(tokens[ignition+1:peak], []byte("/"))

	if len(enter) > 0 {
		if _, err := training.engine.Train(
			enter, []byte(cognition.ActionEnter), feedback,
		); err != nil {
			return errnie.Error(err)
		}
	}

	if len(hold) > 0 {
		if _, err := training.engine.Train(
			hold, []byte(cognition.ActionExit), feedback,
		); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
frames reads the excursion's signal and logic tape from the excursion start up to
the peak and encodes one region token per tick from all signal and logic steps.
*/
func (training *Training) frames(
	detection *data.Measurement[float64], startTick, highTick int64,
) ([]int64, [][]byte, error) {
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

/*
Detect scans stored trade tape for excursions and stores each detection.
*/
func (training *Training) Detect() {
	go func() {
		training.detector.Scan(
			training.catalog.Trades(training.Context()),
		)
	}()
}
