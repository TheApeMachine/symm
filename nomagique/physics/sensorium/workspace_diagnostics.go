package sensorium

import (
	"fmt"
	"math"
)

func (fluid *workspace) sampleWaveDensity(x []float32) (float64, error) {
	pos := [3]float64{float64(x[0]), float64(x[1]), float64(x[2])}
	dims := [3]int{fluid.domain.GridX, fluid.domain.GridY, fluid.domain.GridZ}
	re, _, err := samplePeriodicTrilinear(fluid.psiRe.Float32Slice(), pos, dims, fluid.domain.GridSpacing())
	if err != nil {
		return 0, err
	}
	im, _, err := samplePeriodicTrilinear(fluid.psiIm.Float32Slice(), pos, dims, fluid.domain.GridSpacing())
	if err != nil {
		return 0, err
	}
	return re*re + im*im, nil
}

func (fluid *workspace) accountWaveHead(head int, oldRe, oldIm []float32) error {
	n := int(fluid.domain.MaxModes)
	acc := fluid.accums.Float32Slice()
	ledger := fluid.waveLedger.Float32Slice()
	potential := make([]float32, n)
	for i := 0; i < n; i++ {
		for j := 0; j < 6; j++ {
			if !finite(float64(acc[8*i+j])) {
				return &CoupledStepError{"coherence accumulation", i, true, "nonfinite accumulator"}
			}
		}
		potential[i] = -acc[8*i+2]
		if fluid.spectralPotential != nil {
			potential[i] += fluid.spectralPotential.Float32Slice()[i]
		}
	}
	dw := fluid.domain.binWidth()
	energy := func(re, im, v []float32) (waveEnergy, error) {
		var metric []float32
		if fluid.spectralMetric != nil {
			metric = fluid.spectralMetric.Float32Slice()[:n]
		}
		return spectralGeometryEnergy(re, im, v, metric, dw, fluid.physics.Units.Hbar, massEff, fluid.rates.gInteraction, 0)
	}
	oldPotential := fluid.previousPotential[head*n : (head+1)*n]
	oldH, err := energy(oldRe, oldIm, oldPotential)
	if err != nil {
		return err
	}
	currentH, err := energy(oldRe, oldIm, potential)
	if err != nil {
		return err
	}
	stages := make([]waveEnergy, 3)
	re, im := make([]float32, n), make([]float32, n)
	for stage := 0; stage < 3; stage++ {
		for i := 0; i < n; i++ {
			re[i] = ledger[6*i+2*stage]
			im[i] = ledger[6*i+2*stage+1]
		}
		stages[stage], err = energy(re, im, potential)
		if err != nil {
			return &CoupledStepError{"GPE stage ledger", stage, true, err.Error()}
		}
	}
	s := &fluid.health.Sources
	referencePotential := append([]float32(nil), fluid.reciprocalPotential.Float32Slice()[:n]...)
	if fluid.spectralPotential != nil {
		for i := range referencePotential {
			referencePotential[i] += fluid.spectralPotential.Float32Slice()[i]
		}
	}
	referenceH, err := energy(oldRe, oldIm, referencePotential)
	if err != nil {
		return err
	}
	// Exact algebraic partition under a declared parameter-first convention:
	// old (positions, parameters) -> old positions/new parameters -> new both.
	s.CoherenceParameterWork += referenceH.total() - oldH.total()
	s.CoherenceMotionPotentialChange += currentH.total() - referenceH.total()
	s.CoherencePotentialWork += currentH.total() - oldH.total()
	s.ConservativeWaveError += stages[0].total() - currentH.total()
	s.CoherenceDampingWork += stages[1].total() - stages[0].total()
	s.CoherenceDriveWork += stages[2].total() - stages[1].total()
	s.CoherenceDampingNorm += stages[1].Norm - stages[0].Norm
	s.CoherenceDriveNorm += stages[2].Norm - stages[1].Norm
	// Readouts sum independent heads. The mean head used for projection is not
	// a replacement of their physical states, and is not added to this budget.
	w := &fluid.health.Wave
	last := stages[2]
	w.Norm += last.Norm
	w.Kinetic += last.Kinetic
	w.Potential += last.Potential
	w.Nonlinear += last.Nonlinear
	w.Chemical += last.Chemical
	copy(oldPotential, potential)
	return nil
}

func (fluid *workspace) measureHealth() error {
	fluid.health.Implementation = PhysicsImplementation{ConservativeRemap: true, ReciprocalSpectralForce: true, NodeCheckedSpaceTimeGuidance: true, ProjectedSpatialWave: true, ExternalFieldDrive: true}
	fluid.engine.Synchronize()
	if err := fluid.validateInputs(); err != nil {
		return err
	}
	if err := fluid.engine.HydroRates(fluid.hydro, fluid.acceleration, fluid.hydroDiagnostics, fluid.hydroStatus, fluid.hydroParams(1)); err != nil {
		return err
	}
	if err := checkFlags("gas health", fluid.hydroStatus, fluid.domain.CellCount()); err != nil {
		return err
	}
	d := fluid.domain
	dx := d.GridSpacing()
	vol := dx * dx * dx
	n := d.CellCount()
	rho, mom, thermal, u, diag := fluid.rho.Float32Slice(), fluid.mom.Float32Slice(), fluid.energy.Float32Slice(), fluid.hydro.Float32Slice(), fluid.hydroDiagnostics.Float32Slice()
	h := GasHealth{MinDensity: math.MaxFloat64, MinPressure: math.MaxFloat64, MinTemperature: math.MaxFloat64}
	for z := 0; z < d.GridZ; z++ {
		for y := 0; y < d.GridY; y++ {
			for x := 0; x < d.GridX; x++ {
				j := x + d.GridX*(y+d.GridY*z)
				i := z + d.GridZ*(y+d.GridY*x)
				r, e := float64(rho[j]), float64(thermal[j])
				k, et := float64(diag[8*i+1]), float64(u[6*i+4])
				h.Mass += r * vol
				h.Internal += e * vol
				h.Kinetic += k * vol
				h.Total += et * vol
				p, t, sound, speed := (d.Gamma-1)*e, 0.0, 0.0, float64(diag[8*i+3])
				if r > 0 {
					t = e / (r * d.CV)
					sound = math.Sqrt(d.Gamma * p / r)
				}
				h.MinDensity = math.Min(h.MinDensity, r)
				h.MinPressure = math.Min(h.MinPressure, p)
				h.MinTemperature = math.Min(h.MinTemperature, t)
				h.MaxSpeed = math.Max(h.MaxSpeed, speed)
				h.MaxSound = math.Max(h.MaxSound, sound)
				if sound > 0 {
					h.MaxMach = math.Max(h.MaxMach, speed/sound)
				} else if speed > 0 {
					h.ColdMovingCells++
				}
				difference := math.Abs(float64(diag[8*i+2])) / math.Max(math.Abs(et), math.SmallestNonzeroFloat32)
				h.DisagreementMean += difference
				h.DisagreementMax = math.Max(h.DisagreementMax, difference)
				if difference > fluid.physics.EtaPressure {
					h.DisagreementCount++
				}
				h.AuxiliaryFraction += float64(diag[8*i+7])
				vorticity := float64(diag[8*i+4])
				h.VorticityRMS += vorticity * vorticity
				h.VorticityMax = math.Max(h.VorticityMax, vorticity)
				var grad [3][3]float64
				c := [3]int{x, y, z}
				dims := [3]int{d.GridX, d.GridY, d.GridZ}
				for a := 0; a < 3; a++ {
					left, right := c, c
					left[a] = wrapIndex(c[a]-1, dims[a])
					right[a] = wrapIndex(c[a]+1, dims[a])
					for b := 0; b < 3; b++ {
						li := left[0] + d.GridX*(left[1]+d.GridY*left[2])
						ri := right[0] + d.GridX*(right[1]+d.GridY*right[2])
						lv := cellVelocity(rho, mom, left[0], left[1], left[2], d.GridX, d.GridY, b)
						rv := cellVelocity(rho, mom, right[0], right[1], right[2], d.GridX, d.GridY, b)
						if r > 0 {
							cv := float64(mom[3*j+b]) / r
							switch {
							case rho[li] > 0 && rho[ri] > 0:
								grad[b][a] = (rv - lv) / (2 * dx)
							case rho[ri] > 0:
								grad[b][a] = (rv - cv) / dx
							case rho[li] > 0:
								grad[b][a] = (cv - lv) / dx
							}
						}
					}
				}
				div := grad[0][0] + grad[1][1] + grad[2][2]
				strain2, power := 0.0, 0.0
				for a := 0; a < 3; a++ {
					h.Momentum[a] += float64(mom[3*j+a]) * vol
					for b := 0; b < 3; b++ {
						strain := .5 * (grad[a][b] + grad[b][a])
						strain2 += strain * strain
						dev := strain
						if a == b {
							dev -= div / 3
						}
						power += 2 * d.Mu * dev * dev
					}
				}
				h.StrainRMS += strain2
				h.StrainMax = math.Max(h.StrainMax, math.Sqrt(strain2))
				h.ViscousPower += power * vol
			}
		}
	}
	h.DisagreementMean /= float64(n)
	h.AuxiliaryFraction /= float64(n)
	h.VorticityRMS = math.Sqrt(h.VorticityRMS / float64(n))
	h.StrainRMS = math.Sqrt(h.StrainRMS / float64(n))
	fluid.health.Gas = h
	_, fluid.health.ParticleThermal, fluid.health.ParticleOscillator, fluid.health.ParticleKinetic, _ = fluid.particleTotals()
	fluid.health.ParticleMaterialTotal = fluid.materialTotal()
	fluid.health.ParticleEnergyDisagreement = fluid.health.ParticleMaterialTotal - fluid.health.ParticleThermal - fluid.health.ParticleKinetic
	re, im := fluid.psiRe.Float32Slice(), fluid.psiIm.Float32Slice()
	norm := 0.0
	for i := 0; i < n; i++ {
		if !finite(float64(re[i])) || !finite(float64(im[i])) {
			return &CoupledStepError{"spatial projection", i, true, "nonfinite wave"}
		}
		norm += vol * (float64(re[i])*float64(re[i]) + float64(im[i])*float64(im[i]))
	}
	fluid.health.Wave.ProjectedNorm = norm
	if !fluid.health.IsFinite() {
		return fmt.Errorf("nonfinite physical-health metric")
	}
	return nil
}

// The phase subflow is a driven overdamped rotor in a frozen effective potential.
// Its bath dissipation and drive work are distinct from oscillator thermal energy.
func (fluid *workspace) accountPhase() error {
	values, prior := fluid.phaseLedger.Float32Slice(), fluid.phasePrior.Float32Slice()
	rate := 0.0
	for i := 0; i < fluid.particles; i++ {
		v := values[6*i : 6*i+6]
		for _, x := range v {
			if !finite(float64(x)) {
				return &CoupledStepError{"phase ledger", i, false, "nonfinite potential/work"}
			}
		}
		u0, u1, u2, u3 := float64(v[0]), float64(v[1]), float64(v[2]), float64(v[3])
		dissipation := u1 - u2
		tol := 32 * math.Ldexp(1, -23) * (math.Abs(u1) + math.Abs(u2))
		if dissipation < -tol {
			return &CoupledStepError{"phase gradient flow", i, true, fmt.Sprintf("potential increased by %g", -dissipation)}
		}
		fluid.health.Sources.PhasePotentialWork += u0 - float64(prior[i])
		fluid.health.Sources.PhaseDriveWork += (u1 - u0) + (u3 - u2)
		fluid.health.Sources.PhaseDissipation += dissipation // tiny negative roundoff is measured, not clamped
		fluid.health.Wave.PhasePotential += u3
		prior[i] = v[3]
		rate = math.Max(rate, float64(v[4]))
	}
	fluid.lastPhaseRate = rate
	if rate*fluid.rates.deltaT > fluid.physics.PhaseRadians*(1+8*math.Ldexp(1, -23)) {
		return &CoupledStepError{"phase resolution", -1, true, "accepted phase path exceeds configured angular accuracy bound"}
	}
	return nil
}
