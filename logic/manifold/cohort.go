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
cohortState aggregates mapped orders that share one spatial cell. vel2 is the
mass-weighted sum of squared velocities so Heat can recover PIC rest-frame
kinetic energy after the mean velocity is removed.
*/
type cohortState struct {
	mass   float64
	posX   float64
	posY   float64
	posZ   float64
	velX   float64
	velY   float64
	velZ   float64
	vel2   float64
	omega  float64
	sine   float64
	cosine float64
	orders int
}

/*
cohortsFromMappedOrders conservatively merges co-located mapped orders into one
oscillator population capped by the GPU carrier budget. Amplitude is particle
mass; Heat is Amplitude times the cohort's velocity-dispersion specific
internal energy so sound speed tracks book kinematics instead of a fixed CV
wall that permanently zeroed Hawkes forcing.
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
			cellX: torusCell(order.posX, config.DomainX, config.GridX),
			cellY: torusCell(order.posY, config.DomainY, config.GridY),
			cellZ: torusCell(order.posZ, config.DomainZ, config.GridZ),
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
		state.vel2 += (order.velX*order.velX +
			order.velY*order.velY +
			order.velZ*order.velZ) * order.mass
		state.omega += order.omega * order.mass
		state.sine += math.Sin(order.phase) * order.mass
		state.cosine += math.Cos(order.phase) * order.mass
		state.orders++
	}

	keys := make([]cohortKey, 0, len(cohorts))

	for key := range cohorts {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(left, right int) bool {
		leftMass := cohorts[keys[left]].mass
		rightMass := cohorts[keys[right]].mass

		if leftMass != rightMass {
			return leftMass > rightMass
		}

		if keys[left].cellX != keys[right].cellX {
			return keys[left].cellX < keys[right].cellX
		}

		if keys[left].cellY != keys[right].cellY {
			return keys[left].cellY < keys[right].cellY
		}

		return keys[left].cellZ < keys[right].cellZ
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

		amplitude := state.mass

		if amplitude <= 0 || math.IsNaN(amplitude) || math.IsInf(amplitude, 0) {
			continue
		}

		oscillators = append(oscillators, pmanifold.Oscillator{
			Phase:     math.Atan2(state.sine, state.cosine),
			Omega:     state.omega / state.mass,
			Amplitude: amplitude,
			PosX:      state.posX / state.mass,
			PosY:      state.posY / state.mass,
			PosZ:      state.posZ / state.mass,
			Heat:      amplitude * state.specificEnergy(),
			VelX:      state.velX / state.mass,
			VelY:      state.velY / state.mass,
			VelZ:      state.velZ / state.mass,
		})
	}

	return oscillators
}

/*
specificEnergy is the PIC rest-frame specific kinetic energy of the cohort:
½(⟨|v|²⟩ − |⟨v⟩|²). Coherent books stay cold so Hawkes impulses retain Courant
headroom; dispersed books heat up and constrain forcing.
*/
func (state *cohortState) specificEnergy() float64 {
	if state == nil || state.mass <= 0 {
		return 0
	}

	meanSquare := state.vel2 / state.mass
	meanX := state.velX / state.mass
	meanY := state.velY / state.mass
	meanZ := state.velZ / state.mass
	variance := meanSquare - (meanX*meanX + meanY*meanY + meanZ*meanZ)

	if variance <= 0 || math.IsNaN(variance) || math.IsInf(variance, 0) {
		return 0
	}

	return 0.5 * variance
}

/*
torusCell maps a domain coordinate onto a wrapped grid index.
*/
func torusCell(
	position float64,
	domain float64,
	grid uint32,
) uint32 {
	if grid == 0 || domain <= 0 {
		return 0
	}

	index := int(math.Floor(position*float64(grid)/domain)) % int(grid)

	if index < 0 {
		index += int(grid)
	}

	return uint32(index)
}
