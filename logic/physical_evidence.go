package logic

import (
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

func (physical *physicalManifold) evidence(
	reading pmanifold.Reading,
	projection pmanifold.Reading,
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
		rho:         rho,
		oscillators: oscillators,
	}, nil
}
