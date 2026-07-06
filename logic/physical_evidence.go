package logic

import (
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

func (physical *physicalManifold) evidence(
	reading pmanifold.Reading,
	projection pmanifold.Reading,
	rhoRows [][]float64,
	rho rhoEvidence,
	oscillators oscillatorEvidence,
) (physicalEvidence, error) {
	return physicalEvidence{
		category:    types.CategoryType("physical_field"),
		strength:    rho.gradient,
		shock:       projection.PressureGradNorm,
		resistance:  projection.ViscosityProxy,
		reading:     reading,
		projection:  projection,
		rhoRows:     rhoRows,
		rho:         rho,
		oscillators: oscillators,
	}, nil
}
