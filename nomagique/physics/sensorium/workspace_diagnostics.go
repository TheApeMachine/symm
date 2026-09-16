package sensorium

import (
	"fmt"
	"math"
)

func (workspace *workspace) sampleWaveDensity(x []float32) (float64, error) {
	pos := [3]float64{float64(x[0]), float64(x[1]), float64(x[2])}
	dims := [3]int{workspace.domain.GridX, workspace.domain.GridY, workspace.domain.GridZ}
	re, _, err := samplePeriodicTrilinear(workspace.psiRe.Float32Slice(), pos, dims, workspace.domain.GridSpacing())
	if err != nil {
		return 0, err
	}
	im, _, err := samplePeriodicTrilinear(workspace.psiIm.Float32Slice(), pos, dims, workspace.domain.GridSpacing())
	if err != nil {
		return 0, err
	}
	return re*re + im*im, nil
}

func (workspace *workspace) accountWaveHead(head int, oldRe, oldIm []float32) error {
	n := int(workspace.domain.MaxModes)
	acc := workspace.accums.Float32Slice()
	ledger := workspace.waveLedger.Float32Slice()
	potential := make([]float32, n)
	for i := 0; i < n; i++ {
		for j := 0; j < 6; j++ {
			if !finite(float64(acc[8*i+j])) {
				return &CoupledStepError{"coherence accumulation", i, true, "nonfinite accumulator"}
			}
		}
		potential[i] = -acc[8*i+2]
		if workspace.spectralPotential != nil {
			potential[i] += workspace.spectralPotential.Float32Slice()[i]
		}
	}
	dw := workspace.domain.binWidth()
	energy := func(re, im, v []float32) (waveEnergy, error) {
		var metric []float32
		if workspace.spectralMetric != nil {
			metric = workspace.spectralMetric.Float32Slice()[:n]
		}
		return spectralGeometryEnergy(re, im, v, metric, dw, workspace.physics.Units.Hbar, massEff, workspace.rates.gInteraction, 0)
	}
	oldPotential := workspace.previousPotential[head*n : (head+1)*n]
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
	s := &workspace.health.Sources
	referencePotential := append([]float32(nil), workspace.reciprocalPotential.Float32Slice()[:n]...)
	if workspace.spectralPotential != nil {
		for i := range referencePotential {
			referencePotential[i] += workspace.spectralPotential.Float32Slice()[i]
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
	w := &workspace.health.Wave
	last := stages[2]
	w.Norm += last.Norm
	w.Kinetic += last.Kinetic
	w.Potential += last.Potential
	w.Nonlinear += last.Nonlinear
	w.Chemical += last.Chemical
	copy(oldPotential, potential)
	return nil
}

func (workspace *workspace) measureHealth() error {
	workspace.health.Implementation = PhysicsImplementation{ConservativeRemap: true, ReciprocalSpectralForce: true, NodeCheckedSpaceTimeGuidance: true, ProjectedSpatialWave: true, ExternalFieldDrive: true}
	workspace.engine.Synchronize()
	if err := workspace.validateInputs(); err != nil {
		return err
	}
	if err := workspace.engine.HydroRates(workspace.hydro, workspace.acceleration, workspace.hydroDiagnostics, workspace.hydroStatus, workspace.hydroParams(1)); err != nil {
		return err
	}
	if err := checkFlags("gas health", workspace.hydroStatus, workspace.domain.CellCount()); err != nil {
		return err
	}
	d := workspace.domain
	dx := d.GridSpacing()
	vol := dx * dx * dx
	n := d.CellCount()
	rho, mom, thermal, u, diag := workspace.rho.Float32Slice(), workspace.mom.Float32Slice(), workspace.energy.Float32Slice(), workspace.hydro.Float32Slice(), workspace.hydroDiagnostics.Float32Slice()
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
				if difference > workspace.physics.EtaPressure {
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
	workspace.health.Gas = h
	_, workspace.health.ParticleThermal, workspace.health.ParticleOscillator, workspace.health.ParticleKinetic, _ = workspace.particleTotals()
	workspace.health.ParticleMaterialTotal = workspace.materialTotal()
	workspace.health.ParticleEnergyDisagreement = workspace.health.ParticleMaterialTotal - workspace.health.ParticleThermal - workspace.health.ParticleKinetic
	re, im := workspace.psiRe.Float32Slice(), workspace.psiIm.Float32Slice()
	norm := 0.0
	for i := 0; i < n; i++ {
		if !finite(float64(re[i])) || !finite(float64(im[i])) {
			return &CoupledStepError{"spatial projection", i, true, "nonfinite wave"}
		}
		norm += vol * (float64(re[i])*float64(re[i]) + float64(im[i])*float64(im[i]))
	}
	workspace.health.Wave.ProjectedNorm = norm
	if !workspace.health.IsFinite() {
		return fmt.Errorf("nonfinite physical-health metric")
	}
	return nil
}

// The phase subflow is a driven overdamped rotor in a frozen effective potential.
// Its bath dissipation and drive work are distinct from oscillator thermal energy.
func (workspace *workspace) accountPhase() error {
	values, prior := workspace.phaseLedger.Float32Slice(), workspace.phasePrior.Float32Slice()
	rate := 0.0
	for i := 0; i < workspace.particles; i++ {
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
		workspace.health.Sources.PhasePotentialWork += u0 - float64(prior[i])
		workspace.health.Sources.PhaseDriveWork += (u1 - u0) + (u3 - u2)
		workspace.health.Sources.PhaseDissipation += dissipation // tiny negative roundoff is measured, not clamped
		workspace.health.Wave.PhasePotential += u3
		prior[i] = v[3]
		rate = math.Max(rate, float64(v[4]))
	}
	workspace.lastPhaseRate = rate
	if rate*workspace.rates.deltaT > workspace.physics.PhaseRadians*(1+8*math.Ldexp(1, -23)) {
		return &CoupledStepError{"phase resolution", -1, true, "accepted phase path exceeds configured angular accuracy bound"}
	}
	return nil
}
