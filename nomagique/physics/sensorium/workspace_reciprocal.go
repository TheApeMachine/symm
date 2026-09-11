package sensorium

import "math"

// applyCoherenceImpulse applies the force conjugate to the GPE's *existing*
// anchored potential. It is separate from prescribed pilot drift. The wave
// phase subflows and these impulses form the same interaction Hamiltonian flow.
// The whole gas/remap/wave/pilot composition retains its declared first order.
func (fluid *workspace) applyCoherenceImpulse(dt float32) error {
	fluid.engine.Synchronize()
	force, velocity, mass := fluid.reciprocalForce.Float32Slice(), fluid.vel.Float32Slice(), fluid.mass.Float32Slice()
	for i := 0; i < fluid.particles; i++ {
		f2 := 0.0
		for a := 0; a < 3; a++ {
			f := float64(force[3*i+a])
			if !finite(f) {
				return &CoupledStepError{"coherence force", i, true, "nonfinite force"}
			}
			f2 += f * f
		}
		displacement := .5 * float64(dt) * float64(dt) * math.Sqrt(f2) / float64(mass[i])
		if displacement > fluid.physics.ParticleCells*fluid.domain.GridSpacing() {
			return &CoupledStepError{"coherence impulse", i, true, "unresolved force displacement"}
		}
	}
	for i := 0; i < fluid.particles; i++ {
		work := 0.0
		for a := 0; a < 3; a++ {
			j := 3*i + a
			old := float64(velocity[j])
			next := float32(old + float64(dt)*float64(force[j])/float64(mass[i]))
			if !finite(float64(next)) {
				return &CoupledStepError{"coherence impulse", i, true, "velocity overflow"}
			}
			work += .5 * float64(mass[i]) * (float64(next) - old) * (float64(next) + old)
			velocity[j] = next
		}
		if err := fluid.materialWork(i, work); err != nil {
			return err
		}
		fluid.health.Sources.CoherenceMechanicalWork += work
	}
	return nil
}
