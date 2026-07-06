package logic

import (
	"math"

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
		if err := physical.deposit(clamp); err != nil {
			return physicalEvidence{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"decision physical: failed to deposit clamp",
				err,
			))
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

	oscillatorState, err := physical.field.Oscillators(frame.oscillators)

	if err != nil {
		return physicalEvidence{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision physical: failed to read oscillators",
			err,
		))
	}

	return physical.evidence(reading, projection, rhoRows, rho, oscillatorState)
}

func (physical *physicalManifold) deposit(clamp fieldClamp) error {
	cellX := physical.cell(clamp.positionX, physical.config.GridX)
	cellY := uint32(clamp.lane) % physical.config.GridY
	cellZ := physical.cell(clamp.positionZ, physical.config.GridZ)

	return physical.solver.DepositCell(
		cellX,
		cellY,
		cellZ,
		clamp.rho,
		clamp.momX,
		clamp.momY,
		clamp.momZ,
		clamp.energy,
	)
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

		out = append(out, oscillator)
	}

	return out
}
