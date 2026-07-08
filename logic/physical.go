package logic

import (
	"math"
	"sort"

	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/signal/compute"
)

type physicalManifold struct {
	config pmanifold.Config
	solver *pmanifold.Solver
	field  *physicalField
}

func newPhysicalManifold() (*physicalManifold, error) {
	config, err := decisionManifoldConfig()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: manifold configuration validation failed",
			err,
		))
	}

	var solver *pmanifold.Solver

	err = compute.WithMetalInit(func() error {
		created, err := pmanifold.NewSolver(config)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"decision physical: failed to create manifold solver",
				err,
			))
		}

		solver = created
		return nil
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to create manifold solver",
			err,
		))
	}

	return &physicalManifold{
		config: config,
		solver: solver,
		field:  newPhysicalField(),
	}, nil
}

func (physical *physicalManifold) Close() {
	if physical == nil || physical.solver == nil {
		return
	}

	physical.solver.Close()
	physical.solver = nil
}

func (physical *physicalManifold) SetControls(
	runtime decisionRuntime,
) error {
	if physical == nil || physical.solver == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: solver is not initialized",
			nil,
		))
	}

	controls, err := runtime.controls()

	if err != nil {
		return err
	}

	return physical.solver.SetControls(controls)
}

func (physical *physicalManifold) Settle(
	frame boundaryFrame,
) (physicalEvidence, error) {
	if physical == nil || physical.solver == nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: solver is not initialized",
			nil,
		))
	}

	if err := physical.solver.ResetDeposits(); err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to reset deposits",
			err,
		))
	}

	for _, clamp := range frame.clamps {
		cellY := uint32(clamp.lane)

		if cellY >= physical.config.GridY {
			return physicalEvidence{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"decision physical: clamp lane exceeds manifold grid",
				nil,
			))
		}

		// The clamp is the carrier for this signal; it injects its measurement
		// into the field as passive mass. Each metric is a mass quantum
		// deposited at a cell whose X/Z is displaced by that metric's magnitude,
		// so a rich measurement spreads a population of deposits across the
		// field instead of a single point. The carrier's aggregate rho/mom/
		// energy scales each quantum by its share of the measurement.
		if err := physical.depositMeasurement(clamp, cellY); err != nil {
			return physicalEvidence{}, err
		}
	}

	if err := physical.solver.SetOscillators(
		physical.wrap(frame.oscillators),
	); err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to set oscillators",
			err,
		))
	}

	reading, err := physical.solver.Step()

	if err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to step solver",
			err,
		))
	}

	if !reading.IsFinite() {
		return physicalEvidence{}, errnie.Err(
			errnie.Validation,
			"decision physical: reading must be finite",
			nil,
		)
	}

	projection, err := physical.solver.ReadProjectionReading()

	if err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to read projection reading",
			err,
		))
	}

	if !projection.IsFinite() {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: projection reading must be finite",
			nil,
		))
	}

	rhoRows, err := physical.solver.ReadRhoProjection()

	if err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to read rho projection",
			err,
		))
	}

	rho, err := physical.field.Rho(rhoRows)

	if err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to read rho",
			err,
		))
	}

	particles, err := physical.solver.ReadOscillators(len(frame.oscillators))

	if err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to read particle state",
			err,
		))
	}

	oscillatorState, err := physical.field.Oscillators(particles)

	if err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to read oscillators",
			err,
		))
	}

	return physical.evidence(
		reading,
		projection,
		rhoRows,
		rho,
		oscillatorState,
		particles,
	)
}

// depositMeasurement injects one carrier's measurement into the field as a
// population of mass quanta — one per metric — rather than a single point
// deposit. Each metric displaces its quantum's X/Z from the clamp's base
// position by the metric's magnitude (mapped into a unit displacement), so a
// measurement with more structure spreads density across more of the field.
// The clamp's aggregate rho/mom/energy is split evenly across the quanta so
// total injected mass is conserved; when a measurement carries no metrics the
// carrier still deposits its aggregate at the base cell.
func (physical *physicalManifold) depositMeasurement(
	clamp fieldClamp,
	cellY uint32,
) error {
	keys := make([]string, 0, len(clamp.metrics))

	for key, value := range clamp.metrics {
		if key == "price" || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}

		keys = append(keys, key)
	}

	sort.Strings(keys)

	quanta := len(keys)

	if quanta == 0 {
		return physical.deposit(
			physical.cell(clamp.positionX, physical.config.GridX),
			cellY,
			physical.cell(clamp.positionZ, physical.config.GridZ),
			clamp.rho,
			clamp.momX,
			clamp.momY,
			clamp.momZ,
			clamp.energy,
		)
	}

	share := 1.0 / float64(quanta)

	for _, key := range keys {
		displacement := scalarUnit(clamp.metrics[key])
		positionX := clampUnit(clamp.positionX + (displacement-0.5))
		positionZ := clampUnit(clamp.positionZ + (displacement-0.5))

		if err := physical.deposit(
			physical.cell(positionX, physical.config.GridX),
			cellY,
			physical.cell(positionZ, physical.config.GridZ),
			clamp.rho*share,
			clamp.momX*share,
			clamp.momY*share,
			clamp.momZ*share,
			clamp.energy*share,
		); err != nil {
			return err
		}
	}

	return nil
}

func (physical *physicalManifold) deposit(
	cellX, cellY, cellZ uint32,
	rho, momX, momY, momZ, energy float64,
) error {
	if err := physical.solver.DepositCell(
		cellX,
		cellY,
		cellZ,
		rho,
		momX,
		momY,
		momZ,
		energy,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to deposit clamp",
			err,
		))
	}

	return nil
}

func clampUnit(value float64) float64 {
	return math.Min(1, math.Max(0, value))
}

// scalarUnit maps an unbounded metric magnitude into (0,1). Values already in
// [0,1] pass through; anything larger is squashed by atan so extreme metrics
// still produce a bounded field displacement.
func scalarUnit(value float64) float64 {
	if value >= 0 && value <= 1 {
		return value
	}

	return 0.5 + math.Atan(value)/math.Pi
}

func (physical *physicalManifold) cell(position float64, width uint32) uint32 {
	if width <= 1 {
		return 0
	}

	if position <= 0 {
		return 0
	}

	last := float64(width - 1)
	if position >= 1 {
		return uint32(last)
	}

	return uint32(math.Round(position * last))
}

func (physical *physicalManifold) wrap(
	oscillators []pmanifold.Oscillator,
) []pmanifold.Oscillator {
	out := make([]pmanifold.Oscillator, 0, len(oscillators))

	for _, oscillator := range oscillators {
		oscillator.PosX = float64(physical.cell(
			oscillator.PosX,
			physical.config.GridX,
		))

		oscillator.PosY = math.Mod(oscillator.PosY, float64(physical.config.GridY))

		oscillator.PosZ = float64(physical.cell(
			oscillator.PosZ,
			physical.config.GridZ,
		))

		oscillator.Heat = oscillator.Omega / (physical.config.Gamma - 1)

		out = append(out, oscillator)
	}

	return out
}
