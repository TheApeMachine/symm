package logic

import (
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

type decisionEvidence struct {
	physical       physicalEvidence
	predictive     predictiveEvidence
	counterfactual counterfactualEvidence
	price          float64
	momentum       float64
	pressure       float64
	at             string
}

type physicalEvidence struct {
	category    types.CategoryType
	strength    float64
	shock       float64
	resistance  float64
	reading     pmanifold.Reading
	projection  pmanifold.Reading
	rho         rhoEvidence
	oscillators oscillatorEvidence
}

type rhoEvidence struct {
	mass     float64
	peak     float64
	entropy  float64
	gradient float64
	centerX  float64
	centerZ  float64
	spreadX  float64
	spreadZ  float64
}

type oscillatorEvidence struct {
	coherence float64
	kinetic   float64
	thermal   float64
	omega     float64
}

type predictiveEvidence struct {
	category   types.CategoryType
	confidence float64
	flow       float64
	stress     float64
	coupling   float64
	baseline   float64
	energy     float64
	surprise   float64
	latent     []float64
}

type counterfactualEvidence struct {
	category     types.CategoryType
	confidence   float64
	strength     float64
	baseline     float64
	uplift       float64
	intervention float64
	beta         float64
	panic        float64
	residual     float64
}
