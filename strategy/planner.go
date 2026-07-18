package strategy

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Planner records the feasible action alternatives for each calibrated forecast
and emits orders only for actions that cross the broker boundary.
*/
type Planner struct {
	ctx         context.Context
	cancel      context.CancelFunc
	status      types.Status
	api         *websocket.API
	desk        *broker.Desk
	instrument  *broker.Instrument
	price       *broker.Price
	balance     *broker.Balance
	uiHub       chan<- []byte
	signals     []types.Signal
	workers     []signalWorker
	analyzer    *logic.Analyzer
	recorder    *audit.Recorder
	allocator   *Allocator
	opportunity *Opportunity
	arbiter     *Arbiter
}

/*
signalWorker is one persistent Measure goroutine with a depth-one job slot so
a slow signal never stacks duplicate cuts.
*/
type signalWorker struct {
	signal types.Signal
	jobs   chan *types.Thesis
	out    chan measured
}

type measured struct {
	name string
	rows []*types.Measurement
}

/*
NewPlanner creates one persistent channel worker per signal. Workers measure a
shared immutable market cut concurrently while Planner alone assembles Thesis.
*/
func NewPlanner(
	ctx context.Context,
	uiHub chan<- []byte,
	api *websocket.API,
	desk *broker.Desk,
	instrument *broker.Instrument,
	price *broker.Price,
	balance *broker.Balance,
	signals []types.Signal,
	analyzer *logic.Analyzer,
	allocator *Allocator,
	recorder *audit.Recorder,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	planner := &Planner{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.READY,
		api:        api,
		desk:       desk,
		instrument: instrument,
		price:      price,
		balance:    balance,
		allocator:  allocator,
		uiHub:      uiHub,
		signals:    signals,
		analyzer:   analyzer,
		recorder:   recorder,
		opportunity: NewOpportunity(
			ctx, cancel, price, recorder, uiHub,
		),
		arbiter: NewArbiter(desk, price),
	}
	planner.arbiter.planner = planner
	planner.workers = make([]signalWorker, 0, len(signals))

	for _, signal := range signals {
		worker := signalWorker{
			signal: signal,
			jobs:   make(chan *types.Thesis, 1),
			out:    make(chan measured, 1),
		}
		planner.workers = append(planner.workers, worker)
		go planner.runWorker(worker)
	}

	return planner
}

/*
runWorker measures one job at a time until Planner.Close cancels the context.
*/
func (planner *Planner) runWorker(worker signalWorker) {
	for {
		select {
		case <-planner.ctx.Done():
			return
		case thesis := <-worker.jobs:
			worker.out <- measured{
				name: signalName(worker.signal),
				rows: worker.signal.Measure(thesis),
			}
		}
	}
}

func (planner *Planner) Initialize() error {
	errnie.Info("initializing planner")
	planner.status = types.READY
	return nil
}

/*
Status reports whether the Planner itself is ready to evaluate evidence.
Boot-stage admission remains a separate concern enforced by Update.
*/
func (planner *Planner) Status() types.Status {
	return planner.status
}

/*
Close cancels pending Planner work during runtime shutdown.
*/
func (planner *Planner) Close() {
	planner.cancel()
}

/*
Update fans one immutable market cut through concurrent signal channels, then
analyzes the single Thesis assembled by Planner's result collector.
*/
func (planner *Planner) Update(frame *types.MarketFrame, tick int64) *types.Thesis {
	thesis := types.NewThesis(planner.uiHub, frame)
	thesis.CrossSection = frame.CrossSection
	thesis.Tick = tick
	started := time.Now()

	errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "measure_begin", map[string]any{
		"signals": len(planner.signals),
		"tickers": len(frame.Tickers),
		"trades":  len(frame.Trades),
		"books":   len(frame.Books),
	}))

	interest := types.FrameInterest(frame)

	if interest == 0 {
		interest = types.StreamAll
	}

	counts := make(map[string]int, len(planner.workers))
	active := make([]signalWorker, 0, len(planner.workers))

	for _, worker := range planner.workers {
		if types.SignalInterest(worker.signal)&interest == 0 {
			continue
		}

		select {
		case worker.jobs <- thesis:
			active = append(active, worker)
		default:
			// Depth-one: still measuring prior cut; skip this tick.
		}
	}

	for _, worker := range active {
		batch := <-worker.out
		counts[batch.name] += len(batch.rows)
		thesis.Measurements = append(thesis.Measurements, batch.rows...)
	}

	planner.publishMeasurements(thesis)

	errnie.Error(audit.Record(planner.recorder, "signal_counts", map[string]any{
		"tick":   thesis.Tick,
		"counts": counts,
		"ns":     time.Since(started).Nanoseconds(),
	}))

	errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "measure_end", map[string]any{
		"measurements": len(thesis.Measurements),
		"ns":           time.Since(started).Nanoseconds(),
	}))

	if planner.analyzer != nil {
		planner.analyzer.Update(thesis)
	}

	return thesis
}

/*
Decide manages open inventory, scores flat-symbol entries, arbitrates slots,
then sizes admitted enters.
*/
func (planner *Planner) Decide(thesis *types.Thesis) *types.Thesis {
	if err := errnie.Error(errnie.Require(map[string]any{
		"thesis":      thesis,
		"opportunity": planner.opportunity,
		"arbiter":     planner.arbiter,
		"price":       planner.price,
		"allocator":   planner.allocator,
	})); err != nil {
		return thesis
	}

	planner.manage(thesis)
	planner.opportunity.Measure(thesis)
	planner.arbiter.Select(thesis)

	if err := planner.allocator.Allocate(thesis); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to allocate",
			err,
		))
	}

	planner.commitRotations(thesis)

	return thesis
}

/*
commitRotations appends rotation exits only after the challenger Books a
Reservation, and demotes unsized enters so Trade never submits naked sells.
*/
func (planner *Planner) commitRotations(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	exits := make([]types.Decision, 0)

	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Action == types.ActionEnter && decision.ReservationID == "" {
			decision.Action = types.ActionNothing
			decision.Displaces = ""
			decision.ProposedQuantity = nil
			decision.ProposedNotional = nil
			thesis.Holdings.Delete(decision.Symbol)

			if decision.Reason == "" {
				decision.Reason = "unsized"
			}

			continue
		}

		if decision.Action != types.ActionEnter ||
			decision.Cause != "rotation" ||
			decision.Displaces == "" ||
			decision.ReservationID == "" {
			continue
		}

		qty := decision.Alternatives["incumbent_qty"]
		mark := decision.Alternatives["incumbent_mark"]

		exits = append(exits, types.Decision{
			Action:  types.ActionExit,
			Symbol:  decision.Displaces,
			At:      decision.At,
			Utility: -decision.Alternatives["exit_cost"],
			Alternatives: map[string]float64{
				"exit": -decision.Alternatives["exit_cost"],
				"hold": decision.Alternatives["hold_incumbent"],
			},
			ProposedQuantity:  decimal.NewFromFloat64(qty),
			ReferencePrice:    decimal.NewFromFloat64(mark),
			ValidThroughEpoch: decision.ValidThroughEpoch,
			Cause:             "rotation",
			Reason:            "displaced by higher-utility challenger " + decision.Symbol,
		})

		thesis.Lifecycle.Store(decision.Displaces, types.LifecycleExitSelected)
	}

	thesis.Decisions = append(thesis.Decisions, exits...)
}

/*
publishMeasurements emits one combined measurement frame after fan-in so the
terminal never replaces a partial signal snapshot with another in the same cut.
*/
func (planner *Planner) publishMeasurements(thesis *types.Thesis) {
	if planner.uiHub == nil || thesis == nil || len(thesis.Measurements) == 0 {
		return
	}

	select {
	case planner.uiHub <- datura.Map[any]{
		"measurements": types.WireMeasurements(thesis.Measurements),
	}.Marshal():
	default:
	}
}

/*
signalName returns a stable short label for audit rows from the concrete signal
type without requiring every signal package to advertise a name method.
*/
func signalName(signal types.Signal) string {
	name := fmt.Sprintf("%T", signal)
	name = strings.TrimPrefix(name, "*")

	if index := strings.LastIndex(name, "."); index >= 0 {
		packageName := name[:index]

		if slash := strings.LastIndex(packageName, "/"); slash >= 0 {
			packageName = packageName[slash+1:]
		}

		return packageName
	}

	return name
}

func (planner *Planner) validate(mandatory map[string]any) error {
	check := map[string]any{
		"desk":       planner.desk,
		"instrument": planner.instrument,
		"price":      planner.price,
		"balance":    planner.balance,
		"allocator":  planner.allocator,
		"uiHub":      planner.uiHub,
		"signals":    planner.signals,
		"analyzer":   planner.analyzer,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
