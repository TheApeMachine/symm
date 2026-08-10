package regulator

import (
	"context"
	"errors"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
SubsystemStatus represents the visual health status of one regulated subsystem.
*/
type SubsystemStatus struct {
	Name        string  `json:"name"`
	Label       string  `json:"label"`
	Health      string  `json:"health"`
	Direction   string  `json:"direction"`
	ValueText   string  `json:"valueText"`
	Explanation string  `json:"explanation"`
	Value       float64 `json:"value"`
}

/*
RegulatorPayload is the wire JSON published for the /regulator UI page.
*/
type RegulatorPayload struct {
	Status     string            `json:"status"`
	Surprise   float64           `json:"surprise"`
	Energy     float64           `json:"energy"`
	PnL        float64           `json:"pnl"`
	Summary    string            `json:"summary"`
	Subsystems []SubsystemStatus `json:"subsystems"`
	Sparkline  []float64         `json:"sparkline"`
}

/*
Solver is a thin wrapper around the Predictive Coding engine found
in nomagique. It is a global regulator, designed to automatically
tune the other SYMM subsystems to maximize survival.

It serves as the top-level control mechanism in the SYMM architecture,
monitoring the overall health and performance of the system and
making high-level adjustments to keep the strategy adaptive and
resilient.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	config        *system.Config
	coder         *learning.ResonanceManifold
	ui            chan []byte
	history       []float64
	initialEquity float64
	semaphore     chan struct{}
	thesis        *types.Thesis
}

/*
NewSolver creates a new instance of Solver tied to the ambient system configuration and broker desk.
*/
func NewSolver(
	ctx context.Context,
	ui chan []byte,
	thesis *types.Thesis,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)
	config := system.Cfg

	learningRate := 0.01

	if config != nil && config.Resonance != nil && config.Resonance.LearningRate > 0 {
		learningRate = config.Resonance.LearningRate
	}

	arch := []int{16, 32, 16}
	coder := learning.NewResonanceManifold(arch, 1, learningRate)

	solver := &Solver{
		ctx:       ctx,
		cancel:    cancel,
		config:    config,
		coder:     coder,
		ui:        ui,
		history:   make([]float64, 0, 30),
		semaphore: make(chan struct{}, 1),
		thesis:    thesis,
	}

	solver.thesis.Subscribe(types.SourceRegulator, solver.semaphore)
	solver.run()
	return solver
}

/*
Status reports solver readiness for System waiter and health checks.
*/
func (solver *Solver) Status() types.Status {
	return types.READY
}

func (solver *Solver) run() {
	go func() {
		for {
			select {
			case <-solver.ctx.Done():
				return
			case <-solver.semaphore:
				errnie.Error(solver.Update(solver.thesis))
			}
		}
	}()
}

/*
Update settles system metrics and financial PnL feedback through the regulator manifold,
tunes system.Config fields, and publishes real-time visual regulator status frames over WebSocket.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if solver == nil || solver.coder == nil || solver.config == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: solver, coder, and config required",
			errors.New("invalid regulator solver"),
		))
	}

	if thesis == nil {
		return nil
	}

	pnlRatio, hasEquity := solver.readFinancialFeedback(thesis)

	if !hasEquity {
		return nil
	}

	metrics := make([]float64, 16)

	if solver.config.Resonance != nil {
		metrics[0] = solver.config.Resonance.LearningRate
	}

	if solver.config.Manifold != nil {
		metrics[1] = float64(solver.config.Manifold.RelaxationSteps) / 100.0
	}

	if solver.config.Risk != nil {
		metrics[2] = solver.config.Risk.UncertaintyScale
	}

	if solver.config.Planner != nil {
		metrics[3] = solver.config.Planner.MaxAllocationFraction
	}

	metrics[4] = pnlRatio

	if solver.config.Planner != nil {
		metrics[5] = solver.config.Planner.MinimumConfidence
	}

	if _, err := solver.coder.SettleFromBatch(metrics, nil); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"regulator: settle failed: "+err.Error(),
			err,
		))
	}

	telemetrySurprise := solver.coder.ReconstructionError()
	energy := solver.coder.Energy()

	if math.IsNaN(telemetrySurprise) || math.IsInf(telemetrySurprise, 0) {
		telemetrySurprise = 0.0
	}

	if math.IsNaN(energy) || math.IsInf(energy, 0) {
		energy = 0.0
	}

	// Financial surprisal: negative PnL increases total surprisal (error), positive PnL reduces it
	financialSurprisal := 0.0

	if pnlRatio < 0 {
		financialSurprisal = math.Abs(pnlRatio) * 5.0
	}

	totalSurprise := telemetrySurprise + financialSurprisal

	solver.recordHistory(totalSurprise)
	solver.applyTuning(totalSurprise, pnlRatio)
	payload := solver.buildPayload(totalSurprise, energy, pnlRatio)

	if solver.ui != nil {
		utils.Publish(solver.ui, datura.NewMap("regulator", payload))
	}

	return nil
}

func (solver *Solver) readFinancialFeedback(
	thesis *types.Thesis,
) (float64, bool) {
	equity, exists := thesis.Equity()

	if !exists || equity.Equity == nil || equity.Equity.Sign() <= 0 {
		return 0, false
	}

	currentEquity := equity.Equity.Float64()

	if solver.initialEquity <= 0 {
		solver.initialEquity = currentEquity
		return 0, true
	}

	return (currentEquity - solver.initialEquity) / solver.initialEquity, true
}

func (solver *Solver) recordHistory(value float64) {
	if len(solver.history) >= 30 {
		solver.history = solver.history[1:]
	}

	solver.history = append(solver.history, value)
}

func (solver *Solver) applyTuning(surprise float64, pnlRatio float64) {
	if solver.config == nil {
		return
	}

	if solver.config.Resonance != nil {
		tunedAlpha := math.Max(0.001, math.Min(1.0, solver.config.Resonance.LearningRate*(1.0+surprise*0.01)))
		solver.config.Resonance.LearningRate = tunedAlpha
		_ = solver.coder.SetAlpha(tunedAlpha)
	}

	if solver.config.Manifold != nil {
		minSteps := solver.config.Manifold.MinSteps
		maxSteps := solver.config.Manifold.MaxSteps

		if minSteps < 1 {
			minSteps = 10
		}

		if maxSteps < minSteps {
			maxSteps = 100
		}

		steps := minSteps + int(math.Round(math.Min(1.0, surprise)*float64(maxSteps-minSteps)))

		if steps > maxSteps {
			steps = maxSteps
		}

		solver.config.Manifold.RelaxationSteps = steps
	}

	if solver.config.Risk != nil {
		solver.config.Risk.UncertaintyScale = 1.0 + surprise*0.5
		solver.config.Risk.DrawdownPadding = 0.005 + surprise*0.01
	}

	if solver.config.Planner != nil {
		skill, hasSkill := solver.coder.TaskSkill()

		if hasSkill {
			solver.config.Planner.MinimumSkill = skill
			baseAllocation := math.Min(0.2, math.Max(0.01, 0.1*skill))

			// If drawdown loss is present, contract capital allocation gate
			if pnlRatio < -0.02 {
				baseAllocation = math.Max(0.0, baseAllocation*(1.0+pnlRatio*5.0))
			}

			solver.config.Planner.MaxAllocationFraction = baseAllocation
		}

		// Dynamically regulate MinimumConfidence (80% default baseline)
		// Under high surprise or drawdown, confidence gate tightens up to 95%.
		// Under calm equilibrium, confidence gate relaxes down to 60%.
		confidenceGate := 0.80 + surprise*0.15 - pnlRatio*0.20

		if confidenceGate > 0.95 {
			confidenceGate = 0.95
		} else if confidenceGate < 0.60 {
			confidenceGate = 0.60
		}

		solver.config.Planner.MinimumConfidence = confidenceGate

		// Dynamically regulate Causal MCTS search parameters
		// High surprise or drawdown elevates CausalAlpha (interventional bias over correlation)
		causalAlpha := 1.0 + surprise*2.0

		if pnlRatio < -0.01 {
			causalAlpha += math.Abs(pnlRatio) * 3.0
		}

		if causalAlpha > 5.0 {
			causalAlpha = 5.0
		}

		solver.config.Planner.CausalAlpha = causalAlpha

		mctsIter := 50 + int(math.Round(surprise*100.0))

		if mctsIter > 200 {
			mctsIter = 200
		}

		solver.config.Planner.MCTSIterations = mctsIter
	}
}

func (solver *Solver) buildPayload(surprise float64, energy float64, pnlRatio float64) RegulatorPayload {
	skill, hasSkill := solver.coder.TaskSkill()
	_, hasPrecision := solver.coder.TaskPrecision()

	status := "healthy"
	summary := "System operating in calm, optimal equilibrium."

	if !hasSkill || !hasPrecision || skill < 0.1 || len(solver.history) < 3 {
		status = "observing"
		summary = "System in warm-up state. Observing initial market telemetry to calibrate predictive precision."
	} else if pnlRatio < -0.05 || surprise > 0.4 {
		status = "strained"
		summary = "Financial drawdown or high surprisal detected. Throttling risk boundaries and contracting allocation."
	} else if pnlRatio < -0.01 || surprise > 0.15 {
		status = "adapting"
		summary = "Market turbulence detected. Regulator actively tuning subsystem parameters."
	}

	subsystems := make([]SubsystemStatus, 0, 6)
	healthState := status

	if solver.config.Resonance != nil {
		subsystems = append(subsystems, SubsystemStatus{
			Name:        "resonance",
			Label:       "Resonance Learning Pace",
			Health:      healthState,
			Direction:   "calibrating",
			ValueText:   formatFloat(solver.config.Resonance.LearningRate, 4),
			Explanation: "Learning rate adapting dynamically to market surprise and financial feedback.",
			Value:       solver.config.Resonance.LearningRate,
		})
	}

	if solver.config.Manifold != nil {
		steps := solver.config.Manifold.RelaxationSteps
		dir := "stable"

		if steps > 25 {
			dir = "expanding"
		} else if status == "observing" {
			dir = "calibrating"
		}

		subsystems = append(subsystems, SubsystemStatus{
			Name:        "manifold",
			Label:       "Physics Relaxation Steps",
			Health:      healthState,
			Direction:   dir,
			ValueText:   formatInt(steps) + " steps",
			Explanation: "Compute allocation expanding to resolve market turbulence.",
			Value:       float64(steps),
		})
	}

	if solver.config.Risk != nil {
		dir := "stable"

		if solver.config.Risk.UncertaintyScale > 1.2 {
			dir = "expanding"
		} else if status == "observing" {
			dir = "calibrating"
		}

		subsystems = append(subsystems, SubsystemStatus{
			Name:        "risk",
			Label:       "Stoploss Breathing Room",
			Health:      healthState,
			Direction:   dir,
			ValueText:   formatFloat(solver.config.Risk.UncertaintyScale, 2) + "x scale",
			Explanation: "Stoploss floor padding adjusting to protect equity from drawdown.",
			Value:       solver.config.Risk.UncertaintyScale,
		})
	}

	if solver.config.Planner != nil {
		dir := "open"

		if status == "observing" || solver.config.Planner.MaxAllocationFraction < 0.05 {
			dir = "restricted"
		}

		subsystems = append(subsystems, SubsystemStatus{
			Name:        "planner",
			Label:       "Capital Allocation Gate",
			Health:      healthState,
			Direction:   dir,
			ValueText:   formatFloat(solver.config.Planner.MaxAllocationFraction*100, 1) + "% max",
			Explanation: "Capital velocity gated by model skill and financial PnL drift.",
			Value:       solver.config.Planner.MaxAllocationFraction,
		})

		confDir := "stable"

		if solver.config.Planner.MinimumConfidence > 0.85 {
			confDir = "tightened"
		} else if solver.config.Planner.MinimumConfidence < 0.75 {
			confDir = "relaxed"
		} else if status == "observing" {
			confDir = "calibrating"
		}

		subsystems = append(subsystems, SubsystemStatus{
			Name:        "confidence",
			Label:       "Resonance Decision Gate",
			Health:      healthState,
			Direction:   confDir,
			ValueText:   formatFloat(solver.config.Planner.MinimumConfidence*100, 1) + "% min",
			Explanation: "Minimum forecast confidence required to admit decision signals.",
			Value:       solver.config.Planner.MinimumConfidence,
		})

		mctsDir := "stable"

		if solver.config.Planner.CausalAlpha > 1.5 {
			mctsDir = "elevated"
		} else if status == "observing" {
			mctsDir = "calibrating"
		}

		subsystems = append(subsystems, SubsystemStatus{
			Name:        "mcts",
			Label:       "Causal MCTS Search Bias",
			Health:      healthState,
			Direction:   mctsDir,
			ValueText:   formatFloat(solver.config.Planner.CausalAlpha, 2) + "x bias",
			Explanation: "Interventional causal bias scaling MCTS UCT trajectory selection under market surprise.",
			Value:       solver.config.Planner.CausalAlpha,
		})
	}

	return RegulatorPayload{
		Status:     status,
		Surprise:   surprise,
		Energy:     energy,
		PnL:        pnlRatio * 100.0,
		Summary:    summary,
		Subsystems: subsystems,
		Sparkline:  solver.history,
	}
}

func formatFloat(val float64, decimals int) string {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "0.0"
	}

	multiplier := math.Pow(10, float64(decimals))
	rounded := math.Round(val*multiplier) / multiplier

	return formatRawFloat(rounded)
}

func formatRawFloat(val float64) string {
	if val == float64(int64(val)) {
		return formatInt(int(val))
	}

	return formatPreciseFloat(val)
}

func formatPreciseFloat(val float64) string {
	intPart := int64(val)
	fracPart := int64(math.Abs(val-float64(intPart)) * 1000)

	if fracPart == 0 {
		return formatInt(int(intPart))
	}

	return formatInt(int(intPart)) + "." + formatInt(int(fracPart))
}

func formatInt(val int) string {
	if val == 0 {
		return "0"
	}

	negative := false

	if val < 0 {
		negative = true
		val = -val
	}

	buf := make([]byte, 0, 10)

	for val > 0 {
		buf = append(buf, byte('0'+(val%10)))
		val /= 10
	}

	if negative {
		buf = append(buf, '-')
	}

	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return string(buf)
}

/*
Close stops the solver context.
*/
func (solver *Solver) Close() error {
	if solver == nil || solver.cancel == nil {
		return nil
	}

	solver.cancel()
	return nil
}
