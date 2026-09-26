package store

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/internal/dependence"
)

/*
	Done fixes the focal market's pair and cohort evidence at its own causal cut.

The last lexically ordered admissible peer matches LEGACY Pairs selection.
*/
func (server *RelationsServer) Done(ctx context.Context, call Relations_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	values, err := result.NewValues(17)

	if err != nil {
		return errnie.Error(err)
	}
	present, err := result.NewPresent(17)

	if err != nil {
		return errnie.Error(err)
	}
	result.SetEpoch(server.epoch)
	result.SetSequence(server.sequence)
	focal := server.paths[server.key]

	if focal == nil {
		return nil
	}
	left := focal.measured
	cohort := relationCohort{}
	var selected dependence.Path
	var estimate dependence.Estimate
	selectedKey := ""
	for _, key := range server.keys {
		if key == server.key {
			continue
		}
		right := server.paths[key].measured
		reading := left.Compare(&right, 0)

		if !reading.Defined || reading.Support < 2 {
			continue
		}
		selected, estimate, selectedKey = right, reading, key
		cohort.admit(reading, right.Rate)
	}

	if selectedKey == "" {
		return nil
	}
	pair := []float64{estimate.Correlation, estimate.Covariance, estimate.Support,
		left.Energy, selected.Energy, left.Rate, selected.Rate,
		float64(len(left.Returns)), float64(len(selected.Returns)), estimate.SharedTime,
		estimate.Support / estimate.SharedTime}
	for index, value := range pair {
		values.Set(index, value)
		present.Set(index, true)
	}
	cohort.publish(values.Set, present.Set)

	if err := result.SetPeer(selectedKey); err != nil {
		return errnie.Error(err)
	}
	result.SetPeerSequence(server.paths[selectedKey].sequence)

	if err := writePricePoints(left.Points, result.NewLeft); err != nil {
		return err
	}
	return writePricePoints(selected.Points, result.NewRight)
}

/* writePricePoints copies only the selected pair into immutable native results. */
func writePricePoints(points []dependence.Point, alloc func(int32) (PricePoint_List, error)) error {
	target, err := alloc(int32(len(points)))

	if err != nil {
		return errnie.Error(err)
	}
	for index, point := range points {
		target.At(index).SetAt(point.At)
		target.At(index).SetValue(point.Value)
	}
	return nil
}

/* relationCohort reduces peers within one cut, never across successive times. */
type relationCohort struct {
	weight, squaredWeight, signed, absolute, energy, count float64
	fisherWeight, fisherMean, fisherM2                     float64
}

func (cohort *relationCohort) admit(reading dependence.Estimate, energy float64) {
	correlation, weight := reading.Correlation, reading.Support

	if correlation < -1 || correlation > 1 {
		return
	}
	cohort.weight += weight
	cohort.squaredWeight += weight * weight
	cohort.signed += weight * correlation
	cohort.absolute += weight * math.Abs(correlation)
	cohort.energy += weight * energy
	cohort.count++

	if correlation <= -1 || correlation >= 1 {
		return
	}
	fisher := math.Atanh(correlation)
	previous := cohort.fisherWeight
	cohort.fisherWeight += weight
	delta := fisher - cohort.fisherMean
	cohort.fisherMean += delta * weight / cohort.fisherWeight
	cohort.fisherM2 += delta * delta * previous * weight / cohort.fisherWeight
}

func (cohort *relationCohort) publish(set func(int, float64), present func(int, bool)) {
	if cohort.weight == 0 {
		return
	}
	for offset, value := range []float64{cohort.signed / cohort.weight, cohort.absolute / cohort.weight,
		cohort.energy / cohort.weight, cohort.count, cohort.weight * cohort.weight / cohort.squaredWeight} {
		set(11+offset, value)
		present(11+offset, true)
	}

	if cohort.fisherWeight == cohort.weight {
		set(16, math.Sqrt(cohort.fisherM2/cohort.fisherWeight))
		present(16, true)
	}
}
