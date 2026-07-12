package manifold

import (
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
PressureTensor is the empirical second central velocity moment per axis.
*/
type PressureTensor struct {
	XX float64 `json:"xx"`
	YY float64 `json:"yy"`
	ZZ float64 `json:"zz"`
	XY float64 `json:"xy"`
	XZ float64 `json:"xz"`
	YZ float64 `json:"yz"`
}

func (tensor PressureTensor) IsotropicScalar() float64 {
	return (tensor.XX + tensor.YY + tensor.ZZ) / 3
}

func (tensor PressureTensor) Trace() float64 {
	return tensor.XX + tensor.YY + tensor.ZZ
}

/*
CellDeposit is one conservative deposition into the Metal grid.
*/
type CellDeposit struct {
	CellX uint32
	CellY uint32
	CellZ uint32
	Rho   float64
	MomX  float64
	MomY  float64
	MomZ  float64
	EInt  float64
}

/*
MomentDepositor maps cohorts into grid cells and conservative moment deposits.
*/
type MomentDepositor struct {
	config *pmanifold.Config
	volume float64
}

/*
ConservationMeasurement compares the visible cohort mass with the population
ledger and carries the propagated binary64 rounding bound for that comparison.
*/
type ConservationMeasurement struct {
	VisibleMass float64
	Residual    float64
	Bound       float64
}

func NewMomentDepositor(config *pmanifold.Config) *MomentDepositor {
	return &MomentDepositor{
		config: config,
		volume: config.CellVolume(),
	}
}

func (depositor *MomentDepositor) Deposits(cohorts []Cohort) []CellDeposit {
	deposits := make([]CellDeposit, 0, len(cohorts))
	totalMass := depositor.VisibleMass(cohorts)

	if totalMass <= 0 {
		return deposits
	}

	for _, cohort := range cohorts {
		if cohort.Mass <= 0 {
			continue
		}

		cellX, cellY, cellZ := depositor.cellFor(cohort.Centroid)
		massFraction := cohort.Mass / totalMass
		rho := massFraction / depositor.volume
		momentum := Coordinate{
			Price: rho * cohort.Velocity.Price,
			Size:  rho * cohort.Velocity.Size,
			Age:   rho * cohort.Velocity.Age,
		}
		traceVariance := cohort.SecondMoment.Price +
			cohort.SecondMoment.Size +
			cohort.SecondMoment.Age
		eInt := 0.5 * massFraction * traceVariance / depositor.volume

		deposits = append(deposits, CellDeposit{
			CellX: cellX,
			CellY: cellY,
			CellZ: cellZ,
			Rho:   rho,
			MomX:  momentum.Price,
			MomY:  momentum.Size,
			MomZ:  momentum.Age,
			EInt:  eInt,
		})
	}

	return deposits
}

func (depositor *MomentDepositor) cellFor(centroid Coordinate) (uint32, uint32, uint32) {
	return depositor.axisCell(centroid.Price, depositor.config.DomainX, depositor.config.GridX),
		depositor.axisCell(centroid.Size, depositor.config.DomainY, depositor.config.GridY),
		depositor.unitCell(centroid.Age, depositor.config.GridZ)
}

func (depositor *MomentDepositor) axisCell(value, domain float64, grid uint32) uint32 {
	if grid == 0 || domain <= 0 {
		return 0
	}

	normalized := min(max(value/domain+0.5, 0), 1)
	return min(uint32(normalized*float64(grid)), grid-1)
}

func (depositor *MomentDepositor) unitCell(value float64, grid uint32) uint32 {
	if grid == 0 {
		return 0
	}

	normalized := min(max(value, 0), 1)
	return min(uint32(normalized*float64(grid)), grid-1)
}

func (depositor *MomentDepositor) VisibleMass(cohorts []Cohort) float64 {
	return depositor.visibleQuantity(cohorts).value
}

func (depositor *MomentDepositor) Conservation(
	accounting PopulationAccounting,
	cohorts []Cohort,
) ConservationMeasurement {
	visible := depositor.visibleQuantity(cohorts)
	residual := visible.Subtract(accounting.roundedFinal())

	return ConservationMeasurement{
		VisibleMass: visible.value,
		Residual:    residual.value,
		Bound:       residual.roundoff,
	}
}

func (depositor *MomentDepositor) visibleQuantity(cohorts []Cohort) roundedQuantity {
	mass := roundedQuantity{}

	for _, cohort := range cohorts {
		mass = mass.Add(roundedQuantity{
			value:    cohort.Mass,
			roundoff: cohort.MassRoundoff,
		})
	}

	return mass
}
