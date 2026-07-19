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
	admit       *Admit
	continuity  Continuity
	evidence    Evidence
	rotate      Rotate
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
	name   string
	rows   []*types.Measurement
	failed bool
	panic  string
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

	rotate := NewRotate()
	evidence := NewEvidence()
	admit := NewAdmit(ctx, balance, desk, rotate)

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
			ctx, cancel, price, balance, recorder, uiHub,
		),
		admit:      admit,
		continuity: NewContinuity(price, balance, desk, rotate, evidence),
		evidence:   evidence,
		rotate:     rotate,
		arbiter:    NewArbiter(desk, price, balance, admit, rotate),
	}
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
runWorker measures one job at a time until Planner.Close cancels the context. A
panicking signal is contained by measure so one faulty signal never kills its
worker or stalls the fan-in.
*/
func (planner *Planner) runWorker(worker signalWorker) {
	for {
		select {
		case <-planner.ctx.Done():
			return
		case thesis := <-worker.jobs:
			worker.out <- planner.measure(worker, thesis)
		}
	}
}

/*
measure runs one signal's Measure, recovering any panic into a failed measured
batch with a durable errnie breadcrumb so the worker stays alive for the next
cut while Update can refuse to publish or analyze a compromised thesis.
*/
func (planner *Planner) measure(
	worker signalWorker,
	thesis *types.Thesis,
) (result measured) {
	result.name = signalName(worker.signal)

	defer func() {
		recovered := recover()

		if recovered == nil {
			return
		}

		result.rows = nil
		result.failed = true
		result.panic = fmt.Sprint(recovered)
		errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("planner: signal %s panicked during measure", result.name),
			nil,
		).With("panic", result.panic))
	}()

	result.rows = worker.signal.Measure(thesis)

	return result
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
analyzes the durable Thesis. Per-tick evidence is replaced in place; Holdings
and Lifecycle created by this Thesis are preserved.
*/
func (planner *Planner) Update(
	thesis *types.Thesis,
	frame *types.MarketFrame,
	tick int64,
) *types.Thesis {
	if thesis == nil {
		thesis = types.NewThesis(planner.uiHub, frame)
	}

	thesis.ResetCut(frame, tick)
	started := time.Now()

	errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "measure_begin", map[string]any{
		"signals": len(planner.signals),
		"tickers": len(frame.Tickers),
		"trades":  len(frame.Trades),
		"books":   len(frame.Books),
	}))

	interest := types.FrameInterest(frame)
	counts := make(map[string]int, len(planner.workers))
	active := make([]signalWorker, 0, len(planner.workers))
	skipped := make([]string, 0)

	for _, worker := range planner.workers {
		if types.SignalInterest(worker.signal)&interest == 0 {
			continue
		}

		select {
		case worker.jobs <- thesis:
			active = append(active, worker)
		default:
			// Depth-one: still measuring prior cut — cut is incomplete.
			skipped = append(skipped, signalName(worker.signal))
		}
	}

	failed := make([]string, 0)

	for _, worker := range active {
		batch := <-worker.out

		if batch.failed {
			failed = append(failed, batch.name)
			continue
		}

		counts[batch.name] += len(batch.rows)
		thesis.Measurements = append(thesis.Measurements, batch.rows...)
	}

	if len(skipped) > 0 || len(failed) > 0 {
		thesis.NoteIncomplete()
		thesis.Measurements = nil
		errnie.Error(audit.Phase(planner.recorder, thesis.Tick, "measure_incomplete", map[string]any{
			"skipped": skipped,
			"failed":  failed,
			"ns":      time.Since(started).Nanoseconds(),
		}))

		return thesis
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
	if err := planner.validate(map[string]any{"thesis": thesis}); err != nil {
		return thesis
	}

	// Stamp fees on every forecast before Continuity so occupied/managing lots
	// stay Eligible(); Measure still skips those symbols for fresh enters.
	planner.opportunity.StampFriction(thesis)
	planner.continuity.Manage(thesis)

	if thesis.Incomplete() {
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: types.ActionNothing,
			Cause:  "measure_incomplete",
			Reason: "interested signal skipped this cut; refuse fresh enters",
		})

		return thesis
	}

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
	if err := planner.validate(map[string]any{"thesis": thesis}); err != nil {
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

		thesis.NoteLifecycle(decision.Displaces, types.LifecycleExitSelected, decision.At)
	}

	thesis.Decisions = append(thesis.Decisions, exits...)
}

/*
publishMeasurements emits one combined measurement frame after fan-in so the
terminal never replaces a partial signal snapshot with another in the same cut.
*/
func (planner *Planner) publishMeasurements(thesis *types.Thesis) {
	if thesis == nil || len(thesis.Measurements) == 0 {
		return
	}

	if err := planner.validate(map[string]any{
		"thesis":       thesis,
		"measurements": thesis.Measurements,
	}); err != nil {
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

/*
validate requires Planner surfaces. Call-site extras (e.g. thesis) merge in via
mandatory. Signals and analyzer stay optional on Decide-only paths.
*/
func (planner *Planner) validate(mandatory map[string]any) error {
	check := map[string]any{
		"desk":        planner.desk,
		"instrument":  planner.instrument,
		"price":       planner.price,
		"balance":     planner.balance,
		"allocator":   planner.allocator,
		"opportunity": planner.opportunity,
		"arbiter":     planner.arbiter,
		"admit":       planner.admit,
		"uiHub":       planner.uiHub,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
