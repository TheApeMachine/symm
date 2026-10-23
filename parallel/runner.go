package parallel

import (
	"context"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Runner is the parallel experiment: three tagged virtual books driven by
predictive coders and a physics field. It never imports or mutates strategy.
*/
type Runner struct {
	*runtime.System
	price  *broker.Price
	grid   *nmstore.Grid
	field  *Field
	phase  *PhaseField
	coders *CoderBank
	ledger *Ledger
	client *store.Client
	hub    io.Writer
	mu     sync.Mutex
}

func NewRunner(
	ctx context.Context,
	price *broker.Price,
	client *store.Client,
	hub io.Writer,
) *Runner {
	cfg := system.Cfg.Parallel

	runner := &Runner{
		System: runtime.NewSystem(ctx, "parallel", price),
		price:  price,
		grid:   nmstore.NewGrid(ctx),
		field:  NewField(),
		phase:  NewPhaseField(),
		coders: NewCoderBank(cfg.Pace),
		ledger: NewLedger(),
		client: client,
		hub:    hub,
	}

	if runner.field != nil && runner.field.InitError() != nil {
		runner.Error(runner.field.InitError())
	}

	runner.restore(client)

	return runner
}

func (runner *Runner) Step(value *data.Value) *data.Value {
	if value == nil || runner == nil {
		return value
	}

	observed, err := runner.grid.Observe(value)
	if err != nil {
		runner.Error(err)
		return value
	}

	bySymbol := map[string][]*nmstore.Region{}
	allRegions := observed

	for _, region := range observed {
		bySymbol[region.Symbol] = append(bySymbol[region.Symbol], region)
	}

	seq := int64(data.Tick.Load())
	runner.field.Step(allRegions, seq, 0)

	symbols := make([]string, 0, len(bySymbol))
	for symbol := range bySymbol {
		symbols = append(symbols, symbol)
	}

	slices.Sort(symbols)

	symbolFrames := make([]map[string]any, 0, len(symbols))
	gateTotals := newGateTotals()
	var vizSymbol string
	var vizCoder CoderReading

	for _, symbol := range symbols {
		regions := bySymbol[symbol]
		mid, _ := runner.midpoint(symbol)

		phase := runner.phase.Observe(symbol, regions, 0)
		physicsFeat := globalPhysicsFeatures(runner.field.Reading(), phase)

		coderRead, err := runner.coders.Step(symbol, value, mid, physicsFeat)
		if err != nil {
			runner.Error(err)
			continue
		}

		surprise := math.Max(coderRead.Control.Surprise, coderRead.Treatment.Surprise)
		phase.Surprise = surprise

		if surprise > 2 {
			runner.field.Step(regions, seq, surprise*0.1)
		}

		hurdle := frictionHurdle(value, symbol)
		gatesControl := evaluateGates(
			runner.field.Reading(),
			runner.field.NullReading(),
			runner.field.DeltaR(),
			phase,
			coderRead.Control.ForecastSign,
			runner.field.Healthy(),
		)
		gatesTreatment := evaluateGates(
			runner.field.Reading(),
			runner.field.NullReading(),
			runner.field.DeltaR(),
			phase,
			coderRead.Treatment.ForecastSign,
			runner.field.Healthy(),
		)

		runner.act(BookControl, symbol, coderRead, gatesControl, hurdle, phase)
		runner.act(BookTreatment, symbol, coderRead, gatesTreatment, hurdle, phase)
		runner.act(BookPhysics, symbol, coderRead, gatesTreatment, hurdle, phase)

		gateTotals.add(gatesTreatment)

		if vizSymbol == "" || symbol == types.Focus() {
			vizSymbol = symbol
			vizCoder = coderRead
		}

		symbolFrames = append(symbolFrames, map[string]any{
			"symbol":       symbol,
			"regime":       regimeLabel(regions),
			"surprise":     surprise,
			"forecastSign": coderRead.Treatment.ForecastSign,
			"curveMean":    coderRead.Treatment.CurveMean,
			"gates":        gatesTreatment,
			"phase":        phase,
		})

		runner.maybePersist(symbol, seq)
	}

	reading := runner.field.Reading()
	summary := map[string]any{
		"tick": data.Tick.Load(),
		"books": map[string]any{
			"A": runner.ledger.Stats(BookControl),
			"B": runner.ledger.Stats(BookTreatment),
			"C": runner.ledger.Stats(BookPhysics),
		},
		"deltaBA": runner.ledger.Stats(BookTreatment).MeanReturn - runner.ledger.Stats(BookControl).MeanReturn,
		"physics": map[string]any{
			"healthy":       runner.field.Healthy(),
			"kuramotoR":     reading.KuramotoR,
			"kuramotoPsi":   reading.KuramotoPsi,
			"deltaR":        runner.field.DeltaR(),
			"guidanceSpeed": reading.GuidanceSpeed,
			"coherenceMag2": reading.CoherenceMag2,
			"nullR":         runner.field.NullReading().KuramotoR,
			"spectral":      runner.field.SpectralHint(),
		},
		"gates":   gateTotals.rates(),
		"symbols": symbolFrames,
	}

	value.Append(summary, "parallel", "summary")
	runner.publish(summary)
	runner.publishVisualization(value, vizSymbol, vizCoder)

	return value
}

type gateTotals struct {
	symbols   int
	coherence int
	familiar  int
	guidance  int
	metabolic int
	all       int
}

func newGateTotals() *gateTotals {
	return &gateTotals{}
}

func (totals *gateTotals) add(gates GateSnapshot) {
	totals.symbols++
	if gates.Coherence {
		totals.coherence++
	}

	if gates.Familiar {
		totals.familiar++
	}

	if gates.Guidance {
		totals.guidance++
	}

	if gates.Metabolic {
		totals.metabolic++
	}

	if gates.AllPass {
		totals.all++
	}
}

func (totals *gateTotals) rates() map[string]any {
	if totals.symbols == 0 {
		return map[string]any{"symbols": 0}
	}

	n := float64(totals.symbols)

	return map[string]any{
		"symbols":   totals.symbols,
		"coherence": float64(totals.coherence) / n,
		"familiar":  float64(totals.familiar) / n,
		"guidance":  float64(totals.guidance) / n,
		"metabolic": float64(totals.metabolic) / n,
		"all":       float64(totals.all) / n,
	}
}

func (runner *Runner) act(
	book BookID,
	symbol string,
	read CoderReading,
	gates GateSnapshot,
	hurdle float64,
	phase SymbolPhase,
) {
	side := read.ForBook(book)

	if runner.ledger.HasOpen(book, symbol) {
		if shouldExit(side, phase, book) {
			runner.close(book, symbol)
		}

		return
	}

	switch book {
	case BookPhysics:
		if !physicsOnlyEnter(phase, gates) {
			return
		}
	case BookControl:
		if !skillReady(side) || !curveClearsFriction(side.CurveMean, hurdle) {
			return
		}
	case BookTreatment:
		if !skillReady(side) || !curveClearsFriction(side.CurveMean, hurdle) || !gates.AllPass {
			return
		}
	}

	runner.open(book, symbol)
}

func shouldExit(side CoderSide, phase SymbolPhase, book BookID) bool {
	if book == BookPhysics {
		return phase.DeltaR < -0.01 || phase.Guidance < 0
	}

	if side.ForecastSign < 0 {
		return true
	}

	return phase.DeltaR < -0.005
}

func (runner *Runner) open(book BookID, symbol string) {
	if _, ok := runner.midpoint(symbol); !ok {
		return
	}

	allocation, err := runner.price.AllocateEntry(symbol)
	if err != nil {
		if errnie.IsNotFound(err) {
			return
		}

		runner.Error(err)
		return
	}

	runner.ledger.Open(book, &Lot{
		ID:        uuid.NewString(),
		Symbol:    symbol,
		Book:      book,
		EntryPath: fmt.Sprintf("parallel/%s", book),
		Qty:       allocation.Quantity,
		Entry:     allocation.EntryPrice,
		Fee:       allocation.EntryFee,
	})
}

func (runner *Runner) close(book BookID, symbol string) {
	lot := runner.ledger.OpenLot(book, symbol)
	if lot == nil {
		return
	}

	_, _, ret, ok := runner.price.Exit(symbol, lot.Qty, lot.Entry, lot.Fee)
	if !ok {
		return
	}

	runner.ledger.Close(book, symbol, lot.EntryPath+"/exit", ret)
	runner.persistSymbol(symbol)
	runner.persistLedger()
}

func (runner *Runner) midpoint(symbol string) (float64, bool) {
	if runner.price == nil {
		return 0, false
	}

	bid, ask := runner.price.Touch(symbol)
	if bid == nil || ask == nil {
		return 0, false
	}

	mid := (bid.Float64() + ask.Float64()) / 2

	return mid, mid > 0
}

func (runner *Runner) publish(summary map[string]any) {
	if runner.hub == nil {
		return
	}

	frame := data.NewValue("parallel", "summary", "aggregate")
	frame.Append(summary, "parallel")
	io.Copy(runner.hub, frame)
}
