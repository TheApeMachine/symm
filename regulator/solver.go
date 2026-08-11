package regulator

import (
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"sync"

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
	mu            sync.Mutex
	configSource  *system.Config
	config        *system.Config
	coder         *learning.ResonanceManifold
	pace          *learning.PaceController
	ui            chan []byte
	history       []float64
	lastEquity    float64
	peakEquity    float64
	allocation    float64
	confidence    float64
	causalAlpha   float64
	iterations    int
	learningRate  float64
	relaxation    float64
	uncertainty   float64
	surpriseRank  float64
	rankReady     bool
}

/*
NewSolver creates a new instance of Solver tied to the ambient system configuration and broker desk.
*/
func NewSolver(
	ctx context.Context,
	ui chan []byte,
) *Solver {
	_ = ctx
	configSource := system.Cfg
	config := configSource.Snapshot()

	learningRate := 0.01

	if config != nil && config.Resonance != nil && config.Resonance.LearningRate > 0 {
		learningRate = config.Resonance.LearningRate
	}

	arch := []int{6, 12, 6}
	coder := learning.NewResonanceManifold(arch, 1, learningRate)
	coder.SetStreamLearn(true)

	solver := &Solver{
		configSource: configSource,
		config:       config,
		coder:        coder,
		pace: learning.NewPaceController(learning.PaceConfig{
			InitialAlpha: learningRate,
		}),
		ui:      ui,
		history: make([]float64, 0, 30),
	}

	if config != nil && config.Planner != nil {
		solver.allocation = config.Planner.MaxAllocationFraction
		solver.confidence = config.Planner.MinimumConfidence
		solver.causalAlpha = config.Planner.CausalAlpha
		solver.iterations = config.Planner.MCTSIterations
	}

	if config != nil && config.Resonance != nil {
		solver.learningRate = config.Resonance.LearningRate
	}

	if config != nil && config.Manifold != nil {
		solver.relaxation = float64(config.Manifold.RelaxationSteps)
	}

	if config != nil && config.Risk != nil {
		solver.uncertainty = config.Risk.UncertaintyScale
	}

	return solver
}

/*
Status reports solver readiness for System waiter and health checks.
*/
func (solver *Solver) Status() types.Status {
	return types.READY
}

/*
Update settles system metrics and financial PnL feedback through the regulator manifold,
tunes system.Config fields, and publishes real-time visual regulator status frames over WebSocket.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if solver == nil || solver.coder == nil || solver.pace == nil ||
		solver.config == nil || solver.configSource == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: solver, coder, pace, and config required",
			errors.New("invalid regulator solver"),
		))
	}

	if thesis == nil {
		return nil
	}

	if solver.learningRate <= 0 || solver.relaxation <= 0 ||
		solver.uncertainty <= 0 || solver.allocation <= 0 ||
		solver.confidence <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: positive metric baselines required",
			nil,
		))
	}

	solver.mu.Lock()
	defer solver.mu.Unlock()

	periodReturn, drawdown, hasEquity := solver.readFinancialFeedback(thesis)

	if !hasEquity {
		return nil
	}

	metrics := make([]float64, 6)

	if solver.config.Resonance != nil {
		metrics[0] = solver.config.Resonance.LearningRate / solver.learningRate
	}

	if solver.config.Manifold != nil {
		metrics[1] = float64(solver.config.Manifold.RelaxationSteps) / solver.relaxation
	}

	if solver.config.Risk != nil {
		metrics[2] = solver.config.Risk.UncertaintyScale / solver.uncertainty
	}

	if solver.config.Planner != nil {
		metrics[3] = solver.config.Planner.MaxAllocationFraction / solver.allocation
	}

	metrics[4] = periodReturn

	if solver.config.Planner != nil {
		metrics[5] = solver.config.Planner.MinimumConfidence / solver.confidence
	}

	if _, err := solver.coder.SettleFromBatch(metrics, []float64{periodReturn}); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"regulator: settle failed: "+err.Error(),
			err,
		))
	}

	telemetrySurprise := solver.coder.ReconstructionError()
	energy := solver.coder.Energy()
	totalSurprise := telemetrySurprise + math.Max(0, -drawdown)
	pace, err := solver.pace.Measure(totalSurprise)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"regulator: pace measurement failed",
			err,
		))
	}

	if err := solver.coder.SetAlpha(pace.Alpha); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"regulator: pace update failed",
			err,
		))
	}

	solver.config.Resonance.LearningRate = pace.Alpha
	solver.surpriseRank = pace.Rank
	solver.rankReady = pace.Ready

	solver.recordHistory(totalSurprise)

	if err := solver.applyTuning(); err != nil {
		return err
	}

	payload := solver.buildPayload(totalSurprise, energy, drawdown)

	if solver.ui != nil {
		utils.Publish(solver.ui, datura.NewMap("regulator", payload))
	}

	return nil
}

func (solver *Solver) readFinancialFeedback(
	thesis *types.Thesis,
) (float64, float64, bool) {
	equity, exists := thesis.Equity()

	if !exists || equity.Equity == nil || equity.Equity.Sign() <= 0 {
		return 0, 0, false
	}

	currentEquity := equity.Equity.Float64()

	if solver.lastEquity <= 0 {
		solver.lastEquity = currentEquity
		solver.peakEquity = currentEquity
		return 0, 0, true
	}

	periodReturn := (currentEquity - solver.lastEquity) / solver.lastEquity
	solver.lastEquity = currentEquity

	if currentEquity > solver.peakEquity {
		solver.peakEquity = currentEquity
	}

	drawdown := (currentEquity - solver.peakEquity) / solver.peakEquity
	return periodReturn, drawdown, true
}

func (solver *Solver) recordHistory(value float64) {
	if len(solver.history) >= 30 {
		solver.history = solver.history[1:]
	}

	solver.history = append(solver.history, value)
}

func (solver *Solver) applyTuning() error {
	if solver.config == nil || solver.config.Resonance == nil ||
		solver.config.Planner == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"regulator: resonance and planner configuration required",
			nil,
		))
	}

	skill, hasSkill := solver.coder.TaskSkill()
	precision, hasPrecision := solver.coder.TaskPrecision()

	if hasSkill && hasPrecision {
		if precision <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"regulator: task precision must be strictly positive",
				nil,
			))
		}

		solver.config.Planner.MinimumSkill = skill
		solver.config.Planner.MaxAllocationFraction = solver.allocation *
			math.Min(1, math.Max(0, skill))
		solver.config.Planner.MinimumConfidence = math.Min(
			1,
			solver.confidence/math.Min(1, precision),
		)
	}

	if solver.rankReady {
		tailProbability := math.Abs(1 - solver.surpriseRank)
		solver.config.Planner.CausalAlpha = solver.causalAlpha + tailProbability
		solver.config.Planner.MCTSIterations = max(
			1,
			int(math.Ceil(float64(solver.iterations)*solver.surpriseRank)),
		)
	}

	if err := solver.configSource.ApplyRegulation(
		*solver.config.Resonance,
		*solver.config.Planner,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"regulator: publish tuned configuration failed",
			err,
		))
	}

	return nil
}

func (solver *Solver) buildPayload(surprise float64, energy float64, drawdown float64) RegulatorPayload {
	skill, hasSkill := solver.coder.TaskSkill()
	_, hasPrecision := solver.coder.TaskPrecision()

	status := "healthy"
	summary := "System operating in calm, optimal equilibrium."

	if !hasSkill || !hasPrecision || !solver.rankReady {
		status = "observing"
		summary = "System in warm-up state. Observing initial market telemetry to calibrate predictive precision."
	} else if solver.surpriseRank <= 1/math.Sqrt(float64(solver.pace.Count())) {
		status = "strained"
		summary = "Financial drawdown or high surprisal detected. Throttling risk boundaries and contracting allocation."
	} else if solver.surpriseRank < 0.5 {
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

		if float64(steps) > solver.relaxation {
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

		if solver.config.Risk.UncertaintyScale > solver.uncertainty {
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

		if status == "observing" ||
			solver.config.Planner.MaxAllocationFraction < solver.allocation {
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

		if solver.config.Planner.MinimumConfidence > solver.confidence {
			confDir = "tightened"
		} else if solver.config.Planner.MinimumConfidence < solver.confidence {
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

		if solver.config.Planner.CausalAlpha > solver.causalAlpha {
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
		PnL:        drawdown * 100.0,
		Summary:    summary,
		Subsystems: subsystems,
		Sparkline:  slices.Clone(solver.history),
	}
}

func formatFloat(val float64, decimals int) string {
	return strconv.FormatFloat(val, 'f', decimals, 64)
}

func formatInt(val int) string {
	return strconv.Itoa(val)
}

/*
Close satisfies the solver lifecycle contract; Solver owns no background work.
*/
func (solver *Solver) Close() error {
	return nil
}
