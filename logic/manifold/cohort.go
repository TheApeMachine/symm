package manifold

import (
	"math"
	"sort"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
cohortKey identifies one spatial grid cell in the toroidal manifold domain.
*/
type cohortKey struct {
	cellX uint32
	cellY uint32
	cellZ uint32
}

/*
cohortState aggregates mapped orders that share one spatial cell.
*/
type cohortState struct {
	mass   float64
	posX   float64
	posY   float64
	posZ   float64
	velX   float64
	velY   float64
	velZ   float64
	heat   float64
	omega  float64
	phase  float64
	orders int
}

/*
cohortsFromMappedOrders conservatively merges co-located mapped orders into one
oscillator population capped by the GPU carrier budget.
*/
func cohortsFromMappedOrders(
	config pmanifold.Config,
	orders []mappedOrder,
) []pmanifold.Oscillator {
	if len(orders) == 0 {
		return nil
	}

	cohorts := map[cohortKey]*cohortState{}

	for _, order := range orders {
		key := cohortKey{
			cellX: torusCell(config, order.posX, config.DomainX, config.GridX),
			cellY: torusCell(config, order.posY, config.DomainY, config.GridY),
			cellZ: torusCell(config, order.posZ, config.DomainZ, config.GridZ),
		}

		state := cohorts[key]

		if state == nil {
			state = &cohortState{}
			cohorts[key] = state
		}

		state.mass += order.mass
		state.posX += order.posX * order.mass
		state.posY += order.posY * order.mass
		state.posZ += order.posZ * order.mass
		state.velX += order.velX * order.mass
		state.velY += order.velY * order.mass
		state.velZ += order.velZ * order.mass
		state.heat += order.heat
		state.omega += order.omega * order.mass
		state.phase += order.phase * order.mass
		state.orders++
	}

	keys := make([]cohortKey, 0, len(cohorts))

	for key := range cohorts {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(left, right int) bool {
		return cohorts[keys[left]].mass > cohorts[keys[right]].mass
	})

	limit := int(config.MaxModes)

	if limit <= 0 || limit > len(keys) {
		limit = len(keys)
	}

	oscillators := make([]pmanifold.Oscillator, 0, limit)

	for _, key := range keys[:limit] {
		state := cohorts[key]

		if state.mass <= 0 {
			continue
		}

		amplitude := math.Sqrt(state.mass)

		if amplitude <= 0 || math.IsNaN(amplitude) {
			continue
		}

		oscillators = append(oscillators, pmanifold.Oscillator{
			Phase:     state.phase / state.mass,
			Omega:     state.omega / state.mass,
			Amplitude: amplitude,
			PosX:      state.posX / state.mass,
			PosY:      state.posY / state.mass,
			PosZ:      state.posZ / state.mass,
			Heat:      state.heat,
			VelX:      state.velX / state.mass,
			VelY:      state.velY / state.mass,
			VelZ:      state.velZ / state.mass,
		})
	}

	return oscillators
}

func torusCell(
	config pmanifold.Config,
	position float64,
	domain float64,
	grid uint32,
) uint32 {
	if grid == 0 || domain <= 0 {
		return 0
	}

	index := int(math.Floor(position * float64(grid) / domain))

	if index < 0 {
		index = 0
	}

	maxIndex := int(grid) - 1

	if index > maxIndex {
		index = maxIndex
	}

	return uint32(index)
}
