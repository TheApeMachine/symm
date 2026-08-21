package manifold

const (
	domainGrid      uint32  = 64
	domainExtent    float64 = 1
	domainModes     uint32  = 32
	domainGamma     float64 = 5.0 / 3.0
	domainCFL       float64 = 0.4
	domainViscosity float64 = 1e-4
	domainPrandtl   float64 = 0.71
	domainRhoMin    float64 = 1e-3
	domainPMin      float64 = 1e-3
	domainOmegaMin  float64 = -1
	domainOmegaMax  float64 = 1
	spectralHeads           = 8
	modeAnchors             = 8
	gInteraction            = -1.0
	metabolicRate           = 0.5
	hbarEff                 = 1.0
	massEff                 = 1.0
)

/*
Domain is the market's unit box: grid topology, ω-lattice span, and the
omega-natural gas coefficients (c_v = 1, R = γ-1).
*/
type Domain struct {
	GridX, GridY, GridZ       uint32
	DomainX, DomainY, DomainZ float64
	DeltaT                    float64
	MaxModes                  uint32
	Gamma                     float64
	CV                        float64
	RSpecific                 float64
	RhoMin                    float64
	PMin                      float64
	Mu                        float64
	KThermal                  float64
	OmegaMin                  float64
	OmegaMax                  float64
}

func liveDomain(deltaT float64) Domain {
	spacing := domainExtent / float64(domainGrid)
	rSpecific := domainGamma - 1
	cP := domainGamma * rSpecific / (domainGamma - 1)

	return Domain{
		GridX:     domainGrid,
		GridY:     domainGrid,
		GridZ:     domainGrid,
		DomainX:   float64(domainGrid) * spacing,
		DomainY:   float64(domainGrid) * spacing,
		DomainZ:   float64(domainGrid) * spacing,
		DeltaT:    deltaT,
		MaxModes:  domainModes,
		Gamma:     domainGamma,
		CV:        1,
		RSpecific: rSpecific,
		RhoMin:    domainRhoMin,
		PMin:      domainPMin,
		Mu:        domainViscosity,
		KThermal:  (domainViscosity * cP) / domainPrandtl,
		OmegaMin:  domainOmegaMin,
		OmegaMax:  domainOmegaMax,
	}
}

func (domain Domain) GateWidthMin() float64 {
	return domain.OmegaMin
}

func (domain Domain) GateWidthMax() float64 {
	return domain.OmegaMax
}

func (domain Domain) GridSpacing() float64 {
	maximumAxis := max(domain.GridX, domain.GridY, domain.GridZ)

	if maximumAxis == 0 {
		return 0
	}

	return max(domain.DomainX, domain.DomainY, domain.DomainZ) / float64(maximumAxis)
}

func (domain Domain) CellCount() int {
	return int(domain.GridX * domain.GridY * domain.GridZ)
}

func (domain Domain) binWidth() float64 {
	if domain.MaxModes < 2 {
		return 0
	}

	return (domain.OmegaMax - domain.OmegaMin) / float64(domain.MaxModes-1)
}

func (domain Domain) linewidthMin() float64 {
	domega := domain.binWidth()

	if domega > 0 {
		return 0.25 * domega
	}

	return domain.OmegaMin
}

func (domain Domain) linewidthMax() float64 {
	domega := domain.binWidth()

	if domega > 0 {
		return 4 * domega
	}

	return domain.OmegaMax
}

func (domain Domain) invDomega2() float64 {
	domega := domain.binWidth()

	if domega == 0 {
		return 0
	}

	return 1 / (domega * domega)
}

func (domain Domain) GInteraction() float64 {
	return gInteraction
}

func (domain Domain) AdvectiveDeltaT(driveBeta float64) float64 {
	if !(driveBeta > 0) {
		return 0
	}

	return domainCFL / driveBeta
}
