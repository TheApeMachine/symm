package strategy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
)

/*
Planner records the feasible action alternatives for each calibrated forecast
and emits orders only for actions that cross the broker boundary.
*/
type Planner struct {
	ctx      context.Context
	cancel   context.CancelFunc
	status   types.Status
	uiHub    chan<- []byte
	signals  []types.Signal
	analyzer *logic.Analyzer
	recorder *audit.Recorder
}

/*
SetRecorder attaches a diagnostic recorder so measure can report per-signal
measurement counts. A nil recorder disables recording.
*/
func (planner *Planner) SetRecorder(recorder *audit.Recorder) {
	planner.recorder = recorder

	if planner.analyzer != nil {
		planner.analyzer.SetRecorder(recorder)
	}
}

/*
NewPlanner creates one persistent channel worker per signal. Workers measure a
shared immutable market cut concurrently while Planner alone assembles Thesis.
*/
func NewPlanner(
	ctx context.Context,
	uiHub chan<- []byte,
	signals []types.Signal,
	analyzer *logic.Analyzer,
) *Planner {
	ctx, cancel := context.WithCancel(ctx)

	planner := &Planner{
		ctx:      ctx,
		cancel:   cancel,
		status:   types.READY,
		uiHub:    uiHub,
		signals:  signals,
		analyzer: analyzer,
	}

	return planner
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

	// Fan out first so every signal starts before we block on any result, then
	// drain exactly once per signal. Ranging until close deadlocks here because
	// the closer would only run after Update returns.
	type measured struct {
		name string
		rows []*types.Measurement
	}

	fanIn := make(chan measured, len(planner.signals))

	for _, signal := range planner.signals {
		go func() {
			fanIn <- measured{
				name: signalName(signal),
				rows: signal.Measure(thesis),
			}
		}()
	}

	counts := make(map[string]int, len(planner.signals))

	for range planner.signals {
		batch := <-fanIn
		counts[batch.name] += len(batch.rows)
		thesis.Measurements = append(thesis.Measurements, batch.rows...)
	}

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
Decide compares current executable utility for exposed and unexposed symbols.
Normal slots admit any positive-utility entry; reserved slots stay empty until
an opportunity-class entry needs overflow — so a full book can still take an
OXT-style ignition without mediocre names consuming the reserve lane.
*/
func (planner *Planner) Decide(
	thesis *types.Thesis,
	fees map[string]float64,
	available float64,
	normalSlots int,
	reservedSlots int,
) *types.Thesis {
	entries := make([]types.Decision, 0)
	openBySymbol := make(map[string]types.Holding, len(thesis.Positions))

	for _, holding := range thesis.Positions {
		if holding.Symbol == "" || holding.Closed() {
			continue
		}

		openBySymbol[holding.Symbol] = holding
	}

	open := len(openBySymbol)
	ceiling := normalSlots + reservedSlots
	freeNormal := max(0, normalSlots-open)
	freeReserved := max(0, ceiling-open-freeNormal)
	freeTotal := freeNormal + freeReserved
	slotCapacity := ceiling

	for _, forecast := range thesis.Forecasts {
		fee, feeReady := fees[forecast.Symbol]

		if !forecast.Eligible() || !feeReady || fee < 0 {
			continue
		}

		if _, ok := thesis.Lifecycle.Load(forecast.Symbol); !ok {
			thesis.Lifecycle.Store(forecast.Symbol, types.LifecycleShaped)
		}

		if holding, exists := openBySymbol[forecast.Symbol]; exists {
			decision := planner.continuation(forecast, fee, holding)
			decision.Cause = planner.cause(thesis, forecast, decision.Action)
			planner.context(&decision, forecast, available, open, slotCapacity)
			thesis.Decisions = append(thesis.Decisions, decision)

			if decision.Action == "exit" {
				thesis.Lifecycle.Store(forecast.Symbol, types.LifecycleExitSelected)
			}

			if decision.Action == "exit" || decision.Action == "reduce" {
				thesis.Orders = append(thesis.Orders, spot.Order{
					Description: &spot.OrderDescription{
						Pair:      forecast.Symbol,
						Type:      decision.Action,
						Price:     decimal.NewFromFloat64(decision.ReferencePrice),
						OrderType: "market",
					},
					Volume: decimal.NewFromFloat64(decision.ProposedQuantity),
					Price:  decimal.NewFromFloat64(decision.ReferencePrice),
				})
			}

			continue
		}

		cognitionValue, cognitionFound := thesis.Cognition.Load(forecast.Symbol)
		cognition, cognitionValid := cognitionValue.(types.Cognition)

		if !cognitionFound || !cognitionValid || !cognition.Ready ||
			cognition.Ambiguous || cognition.Winner != "buy" {
			reason := "cognitive memory is not ready for this evidence sequence"
			cause := "cognitive_not_ready"

			if cognitionValid && cognition.Ambiguous {
				reason = "cognitive memory is ambiguous for this evidence sequence"
				cause = "cognitive_ambiguity"
			}

			if cognitionValid && cognition.Ready && !cognition.Ambiguous &&
				cognition.Winner != "buy" {
				reason = "cognitive memory does not support a buy entry"
				cause = "cognitive_opposition"
			}

			decision := planner.nothing(forecast, reason)
			decision.Cause = cause
			planner.context(&decision, forecast, available, open, slotCapacity)
			thesis.Decisions = append(thesis.Decisions, decision)

			continue
		}

		if freeTotal <= 0 || available <= 0 {
			decision := planner.nothing(
				forecast, "portfolio capacity makes entry infeasible",
			)
			planner.context(&decision, forecast, available, open, slotCapacity)
			thesis.Decisions = append(thesis.Decisions, decision)

			continue
		}

		decision := planner.entry(
			thesis,
			forecast,
			cognition,
			fee,
			available/float64(freeTotal),
		)
		planner.context(&decision, forecast, available, open, slotCapacity)

		if decision.Action == "nothing" {
			thesis.Decisions = append(thesis.Decisions, decision)

			continue
		}

		entries = append(entries, decision)
	}

	planner.admit(thesis, entries, freeNormal, freeReserved)

	return thesis
}
