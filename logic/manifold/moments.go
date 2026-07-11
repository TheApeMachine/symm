package manifold

import (
	"math"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
PressureTensor is the empirical second central velocity moment per axis.
*/
type PressureTensor struct {
	XX float64
	YY float64
	ZZ float64
	XY float64
	XZ float64
	YZ float64
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
		depositor.axisCell(centroid.Age, depositor.config.DomainZ, depositor.config.GridZ)
}

func (depositor *MomentDepositor) axisCell(value, domain float64, grid uint32) uint32 {
	if grid == 0 || domain <= 0 {
		return 0
	}

	normalized := (value/domain + 0.5)

	if normalized < 0 {
		normalized = 0
	}

	if normalized > 1 {
		normalized = 1
	}

	index := uint32(math.Min(float64(grid-1), math.Floor(normalized*float64(grid))))

	return index
}

func (depositor *MomentDepositor) VisibleMass(cohorts []Cohort) float64 {
	mass := 0.0

	for _, cohort := range cohorts {
		mass += cohort.Mass
	}

	return mass
}

func (depositor *MomentDepositor) ConservationResidual(accounting PopulationAccounting, visibleMass float64) float64 {
	return visibleMass - accounting.Final()
}
