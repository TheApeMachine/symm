package trader

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/* Crypto is the trading runtime that turns ordered market frames into orders. */
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	status       types.Status
	booter       *system.Booter
	api          *websocket.API
	desk         *broker.Desk
	balance      *broker.Balance
	instrument   *broker.Instrument
	tick         int64
	planner      *strategy.Planner
	analyzer     *logic.Analyzer
	uiHub        *ui.Hub
	signals      []types.Signal
	measurements chan []*types.Measurement
	sync         chan chan struct{}
}

/* NewCrypto wires market ingress, signal output, planning, and execution. */
func NewCrypto(
	ctx context.Context,
	booter *system.Booter,
	api *websocket.API,
	desk *broker.Desk,
	balance *broker.Balance,
	instrument *broker.Instrument,
	analyzer *logic.Analyzer,
	uiHub *ui.Hub,
	planner *strategy.Planner,
	signals []types.Signal,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		status:       types.READY,
		booter:       booter,
		api:          api,
		desk:         desk,
		balance:      balance,
		instrument:   instrument,
		analyzer:     analyzer,
		planner:      planner,
		uiHub:        uiHub,
		signals:      signals,
		measurements: make(chan []*types.Measurement, viper.GetInt("signals.feed_timeline_capacity")),
		sync:         make(chan chan struct{}),
	}

	return crypto
}

/*
Initialize starts the ingress and fan-in goroutines. Ingress copies each typed
websocket batch into every signal's channels; the signals compute concurrently
and their measurements collect for the next tick.
*/
func (crypto *Crypto) Initialize() error {
	errnie.Info("initializing crypto")

	go crypto.ingest()

	crypto.status = types.READY
	return nil
}

/*
ingest reads the typed websocket channels and feeds every signal directly. Each
frame is fanned out to all signals and then barriered on their acks, so a frame
is fully measured before the next is dispatched and a drained feed leaves the
measurements ready for the next tick.
*/
func (crypto *Crypto) ingest() {
	measures := make([]chan []*types.Measurement, len(crypto.signals))

	for index, signal := range crypto.signals {
		measures[index] = signal.Measure()
	}

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case reply := <-crypto.sync:
			// Fully process every frame currently queued on the transport
			// channels, then acknowledge so a blocked Tick observes exactly
			// this step's measurements and never a partial or previous cut.
			if !crypto.drainFrames(measures) {
				return
			}

			close(reply)
		case tickers := <-crypto.api.TickerChannel():
			if !crypto.dispatch(measures, func(signal types.Signal) {
				signal.Tickers() <- tickers
			}) {
				return
			}
		case trades := <-crypto.api.TradeChannel():
			if !crypto.dispatch(measures, func(signal types.Signal) {
				signal.Trades() <- trades
			}) {
				return
			}
		case books := <-crypto.api.BookChannel():
			crypto.stampBooks(books)

			if !crypto.dispatch(measures, func(signal types.Signal) {
				signal.Books() <- books
			}) {
				return
			}
		}
	}
}

/*
drainFrames processes every frame still pending on the transport channels until
they are all empty, returning false only if the runtime was cancelled.
*/
func (crypto *Crypto) drainFrames(measures []chan []*types.Measurement) bool {
	for {
		select {
		case <-crypto.ctx.Done():
			return false
		case tickers := <-crypto.api.TickerChannel():
			if !crypto.dispatch(measures, func(signal types.Signal) {
				signal.Tickers() <- tickers
			}) {
				return false
			}
		case trades := <-crypto.api.TradeChannel():
			if !crypto.dispatch(measures, func(signal types.Signal) {
				signal.Trades() <- trades
			}) {
				return false
			}
		case books := <-crypto.api.BookChannel():
			crypto.stampBooks(books)

			if !crypto.dispatch(measures, func(signal types.Signal) {
				signal.Books() <- books
			}) {
				return false
			}
		default:
			return true
		}
	}
}

/*
dispatch fans one frame out to every signal, barriers on their acks, then moves
each emitted measurement batch into the shared collector. A signal emits its
batch before acking, so a non-blocking receive after the ack observes it. ingest
is the sole writer of crypto.measurements, keeping per-frame ordering exact.
Returns false only if the runtime was cancelled.
*/
func (crypto *Crypto) dispatch(
	measures []chan []*types.Measurement,
	fan func(types.Signal),
) bool {
	for _, signal := range crypto.signals {
		fan(signal)
	}

	for index, signal := range crypto.signals {
		select {
		case <-crypto.ctx.Done():
			return false
		case <-signal.Ack():
		}

		select {
		case batch := <-measures[index]:
			select {
			case <-crypto.ctx.Done():
				return false
			case crypto.measurements <- batch:
			}
		default:
		}
	}

	return true
}

/* stampBooks enriches book levels with the instrument tick size before ingress. */
func (crypto *Crypto) stampBooks(books []kraken.BookData) {
	for index := range books {
		pair, err := crypto.instrument.Pair(books[index].Symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		books[index].PriceIncrement = &pair.PriceIncrement
	}
}

/* Status reports whether the runtime can accept production market cuts. */
func (crypto *Crypto) Status() types.Status {
	return crypto.status
}

/* Run plans a cut each time signal measurements arrive after warmup. */
func (crypto *Crypto) Run() error {
	go func() {
		errnie.Info("crypto runtime started")

		for {
			select {
			case <-crypto.ctx.Done():
				return
			case batch := <-crypto.measurements:
				if !crypto.booter.Ready(system.StageWarmup) {
					continue
				}

				_, err := crypto.plan(batch)

				if err == nil || errnie.IsPreconditionFailed(err) {
					continue
				}

				errnie.Error(err)
				crypto.status = types.ERROR
				crypto.cancel()
				return
			}
		}
	}()

	return nil
}

/*
Tick drains any measurements currently queued into a Thesis. It is retained for
callers (tests) that drive the runtime a cut at a time.
*/
func (crypto *Crypto) Tick() (*types.Thesis, error) {
	// Block until ingest has fully processed every frame queued for this cut,
	// so the drain below sees exactly this step's measurements. Without this
	// barrier Tick would race the asynchronous signal pipeline and observe an
	// empty or previous cut.
	reply := make(chan struct{})

	select {
	case <-crypto.ctx.Done():
		return crypto.plan(nil)
	case crypto.sync <- reply:
		select {
		case <-crypto.ctx.Done():
		case <-reply:
		}
	}

	return crypto.plan(nil)
}

/*
tick folds seed plus every currently queued measurement batch into one Thesis.
*/
func (crypto *Crypto) plan(seed []*types.Measurement) (*types.Thesis, error) {
	rows := make([]*types.Measurement, 0, len(seed))
	rows = append(rows, seed...)

	for {
		select {
		case batch := <-crypto.measurements:
			rows = append(rows, batch...)
			continue
		default:
		}

		break
	}

	if len(rows) == 0 {
		return nil, errnie.Err(errnie.PreconditionFailed,
			"crypto: no signal measurements", nil)
	}

	crypto.tick++
	tick := crypto.tick
	at := time.Time{}

	for _, row := range rows {
		if row.At.After(at) {
			at = row.At
		}
	}

	thesis := crypto.planner.Update(nil, at, tick, rows)

	for holding := range crypto.balance.Holdings() {
		phase, found := thesis.Lifecycle.Load(holding.Symbol)

		if holding.Status == types.OPEN && (!found ||
			phase == types.LifecycleEntrySubmitted ||
			phase == types.LifecycleEntered) {
			thesis.NoteLifecycle(holding.Symbol, types.LifecycleManaging, thesis.At)
		}
	}

	thesis.Holdings.Range(func(key, value any) bool {
		holding := value.(*types.Holding)
		closed := false

		if holding.Stoploss != nil {
			holding.Stoploss.RLock()
			closed = holding.Status == types.CLOSED
			holding.Stoploss.RUnlock()
		}

		if closed {
			thesis.NoteLifecycle(key.(string), types.LifecycleClosed, thesis.At)
		}

		return true
	})

	thesis, err := crypto.planner.Decide(thesis)

	if err != nil {
		return thesis, err
	}

	for _, decision := range thesis.Decisions {
		if decision.Action != types.ActionExit {
			continue
		}

		if phase, found := thesis.Lifecycle.Load(decision.Symbol); found &&
			phase == types.LifecycleExitSubmitted {
			continue
		}

		if err := crypto.desk.Sell(decision.Symbol); err != nil {
			errnie.Error(errnie.Err(errnie.Internal,
				"failed to submit exit for "+decision.Symbol, err))
			continue
		}

		thesis.NoteLifecycle(decision.Symbol, types.LifecycleExitSubmitted, thesis.At)
	}

	selected := make([]string, 0)
	thesis.Lifecycle.Range(func(key, value any) bool {
		if value == types.LifecycleEntrySelected {
			selected = append(selected, key.(string))
		}

		return true
	})
	sort.Slice(selected, func(left, right int) bool {
		leftValue, _ := thesis.Holdings.Load(selected[left])
		rightValue, _ := thesis.Holdings.Load(selected[right])
		leftHolding := leftValue.(*types.Holding)
		rightHolding := rightValue.(*types.Holding)

		if leftHolding.IsOpportunity != rightHolding.IsOpportunity {
			return !leftHolding.IsOpportunity
		}

		return selected[left] < selected[right]
	})

	for _, symbol := range selected {
		raw, found := thesis.Holdings.Load(symbol)

		if !found {
			return thesis, errnie.Error(errnie.Err(errnie.Internal,
				"selected entry has no holding for "+symbol, nil))
		}

		holding := raw.(*types.Holding)

		if !crypto.desk.HasSlot(holding.IsOpportunity) {
			continue
		}

		position, err := crypto.desk.Buy(holding, holding.IsOpportunity)

		if err != nil {
			return thesis, errnie.Error(errnie.Err(errnie.Internal,
				"failed to submit entry for "+symbol, err))
		}

		thesis.Positions.Store(symbol, position)
		thesis.NoteLifecycle(symbol, types.LifecycleEntrySubmitted, thesis.At)
	}

	if len(thesis.Decisions) > 0 {
		select {
		case crypto.uiHub.Messages <- datura.Map[any]{"decisions": thesis.Decisions}.Marshal():
		default:
		}
	}

	select {
	case crypto.uiHub.Messages <- datura.Map[any]{"tick": datura.Map[any]{
		"count":        thesis.Tick,
		"measurements": types.ObservationCount(thesis.Measurements),
		"open":         crypto.desk.HoldingCount(),
		"completed":    true,
		"phase":        "complete",
	}}.Marshal():
	default:
	}

	return thesis, nil
}

/* Close joins the runtime before releasing its owned dependencies. */
func (crypto *Crypto) Close() (err error) {
	crypto.cancel()

	err = errors.Join(err, crypto.planner.Close(), crypto.desk.Close(), crypto.analyzer.Close())

	return err
}
