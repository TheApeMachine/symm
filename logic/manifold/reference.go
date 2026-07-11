package manifold

import (
	"math"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
ReferenceDeposits is the deterministic CPU oracle for cohort-to-cell deposition.
It mirrors MomentDepositor without invoking the Metal solver.
*/
func ReferenceDeposits(config *pmanifold.Config, cohorts []Cohort) []CellDeposit {
	if config == nil {
		return nil
	}

	depositor := NewMomentDepositor(config)

	return depositor.Deposits(cohorts)
}

/*
DepositsEqual reports whether two deposit slices match within tolerance.
*/
func DepositsEqual(left, right []CellDeposit, tolerance float64) bool {
	if len(left) != len(right) {
		return false
	}

	if tolerance <= 0 {
		tolerance = 1e-12
	}

	for index := range left {
		if left[index].CellX != right[index].CellX ||
			left[index].CellY != right[index].CellY ||
			left[index].CellZ != right[index].CellZ {
			return false
		}

		if !depositClose(left[index], right[index], tolerance) {
			return false
		}
	}

	return true
}

func depositClose(left, right CellDeposit, tolerance float64) bool {
	return closeFloat(left.Rho, right.Rho, tolerance) &&
		closeFloat(left.MomX, right.MomX, tolerance) &&
		closeFloat(left.MomY, right.MomY, tolerance) &&
		closeFloat(left.MomZ, right.MomZ, tolerance) &&
		closeFloat(left.EInt, right.EInt, tolerance)
}

func closeFloat(left, right, tolerance float64) bool {
	return math.Abs(left-right) <= tolerance
}

/*
ReferencePressureTensor aggregates the full empirical pressure tensor from cohorts.
*/
func ReferencePressureTensor(
	config *pmanifold.Config,
	cohorts []Cohort,
) PressureTensor {
	if config == nil {
		return PressureTensor{}
	}

	volume := config.CellVolume()
	tensor := PressureTensor{}

	for _, cohort := range cohorts {
		if cohort.Mass <= 0 {
			continue
		}

		weight := cohort.Mass / volume

		tensor.XX += weight * cohort.SecondMoment.Price
		tensor.YY += weight * cohort.SecondMoment.Size
		tensor.ZZ += weight * cohort.SecondMoment.Age
		tensor.XY += weight * cohort.CrossMoment.PriceSize
		tensor.XZ += weight * cohort.CrossMoment.PriceAge
		tensor.YZ += weight * cohort.CrossMoment.SizeAge
	}

	return tensor
}
