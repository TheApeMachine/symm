package sensorium

import (
	"fmt"
	"math"
)

// debitHeadBudget debits the original persistent reservoir exactly once. No
// head can borrow the next head's share and no negative state is repaired.
func debitHeadBudget(initial float32, spent float64) (float32, error) {
	if !finite(float64(initial)) || initial < 0 || !finite(spent) || spent < 0 || spent > float64(initial) {
		return 0, fmt.Errorf("invalid head budget Q0=%g spent=%g", initial, spent)
	}
	remaining := float32(float64(initial) - spent)
	if !finite(float64(remaining)) || remaining < 0 {
		return 0, fmt.Errorf("invalid head remainder")
	}
	return remaining, nil
}

type waveEnergy struct{ Norm, Kinetic, Potential, Nonlinear, Chemical float64 }

func (w waveEnergy) total() float64 { return w.Kinetic + w.Potential + w.Nonlinear + w.Chemical }



func spectralGeometryEnergy(real, imag, potential, metric []float32, dw, hbar, inertia, g, mu float64) (waveEnergy, error) {
	h := waveEnergy{}
	n := len(real)
	if n < 1 || len(imag) != n || len(potential) != n || (len(metric) != 0 && len(metric) != n) || !isPositiveFinite(dw) || !isPositiveFinite(hbar) || !isPositiveFinite(inertia) || !finite(g) || !finite(mu) {
		return h, fmt.Errorf("invalid spectral Hamiltonian contract")
	}
	coefficient := hbar * hbar / (2 * inertia * dw * dw)
	for i := 0; i < n; i++ {
		re, im, v := float64(real[i]), float64(imag[i]), float64(potential[i])
		if !finite(re) || !finite(im) || !finite(v) {
			return h, fmt.Errorf("nonfinite spectral state %d", i)
		}
		j := (i + 1) % n
		wi, wj := 1., 1.
		if len(metric) > 0 {
			wi, wj = float64(metric[i]), float64(metric[j])
		}
		if !isPositiveFinite(wi) || !isPositiveFinite(wj) {
			return h, fmt.Errorf("invalid spectral metric at %d", i)
		}
		dr, di := float64(real[j])/math.Sqrt(wj)-re/math.Sqrt(wi), float64(imag[j])/math.Sqrt(wj)-im/math.Sqrt(wi)
		density := re*re + im*im
		h.Norm += dw * density
		h.Kinetic += dw * coefficient * (dr*dr + di*di) / (.5*wi + .5*wj)
		h.Potential += dw * v * density
		h.Nonlinear += dw * .5 * g * density * density / wi
		h.Chemical -= dw * mu * density
	}
	if !finite(h.total()) || !finite(h.Norm) {
		return h, fmt.Errorf("spectral Hamiltonian overflow")
	}
	return h, nil
}
