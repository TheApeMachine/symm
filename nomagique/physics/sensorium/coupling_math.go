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

type pilotReplacement struct {
	Position, Velocity [3]float32
	Work, Speed, Cells float64
}

// Gas has already transported x by v*dt. Only the change in prescribed pilot
// drift is an additional displacement/impulse. Both native backends use this
// shared host coupling operator; there is no backend-selecting CPU fallback.
func replacePilotDrift(x, v, oldGuide, newGuide [3]float32, mass, dt, dx, cellLimit float64, extent [3]float64) (pilotReplacement, error) {
	out := pilotReplacement{}
	if !isPositiveFinite(mass) || !finite(dt) || dt < 0 || !isPositiveFinite(dx) || !isPositiveFinite(cellLimit) {
		return out, fmt.Errorf("invalid pilot coupling parameters")
	}
	var speed2, displacement2 float64
	for a := 0; a < 3; a++ {
		if !finite(float64(x[a])) || !finite(float64(v[a])) || !finite(float64(oldGuide[a])) || !finite(float64(newGuide[a])) || !isPositiveFinite(extent[a]) {
			return out, fmt.Errorf("invalid pilot coupling state on axis %d", a)
		}
		delta := float64(newGuide[a]) - float64(oldGuide[a])
		vi := float64(v[a])
		vn := float32(vi + delta)
		// Use the stored float32 velocity to ledger actual round-off too.
		out.Work += .5 * mass * (float64(vn) - vi) * (float64(vn) + vi)
		out.Velocity[a] = vn
		position := math.Mod(float64(x[a])+dt*delta, extent[a])
		if position < 0 {
			position += extent[a]
		}
		out.Position[a] = float32(position)
		if float64(out.Position[a]) == extent[a] {
			out.Position[a] = 0
		} // rounded periodic endpoint
		if !finite(float64(vn)) || !finite(float64(out.Position[a])) {
			return out, fmt.Errorf("pilot state overflow")
		}
		speed2 += float64(newGuide[a]) * float64(newGuide[a])
		// Sum of operator path lengths, not their cancelling net displacement.
		displacement2 += math.Pow(dt*(math.Abs(vi)+math.Abs(delta))/dx, 2)
	}
	out.Speed = math.Sqrt(speed2)
	out.Cells = math.Sqrt(displacement2)
	if !finite(out.Work) || out.Cells > cellLimit {
		return out, fmt.Errorf("pilot path %.9g cells exceeds %.9g", out.Cells, cellLimit)
	}
	return out, nil
}

// samplePeriodicTrilinear returns value and exact gradient of the z-fast
// periodic trilinear interpolant. This independent host expression is used for
// diagnostics/reference tests, NOT instead of native pilot evaluation.
func samplePeriodicTrilinear(field []float32, position [3]float64, dims [3]int, dx float64) (float64, [3]float64, error) {
	var grad [3]float64
	if !isPositiveFinite(dx) || dims[0] < 1 || dims[1] < 1 || dims[2] < 1 || len(field) != dims[0]*dims[1]*dims[2] {
		return 0, grad, fmt.Errorf("invalid trilinear grid")
	}
	var base [3]int
	var f [3]float64
	for a := 0; a < 3; a++ {
		if !finite(position[a]) {
			return 0, grad, fmt.Errorf("invalid trilinear coordinate")
		}
		q := math.Mod(position[a]/dx, float64(dims[a]))
		if q < 0 {
			q += float64(dims[a])
		}
		base[a] = int(math.Floor(q))
		f[a] = q - float64(base[a])
		if base[a] == dims[a] {
			base[a] = 0
			f[a] = 0
		}
	}
	result := 0.0
	for iz := 0; iz < 2; iz++ {
		for iy := 0; iy < 2; iy++ {
			for ix := 0; ix < 2; ix++ {
				b := [3]int{ix, iy, iz}
				var w [3]float64
				var sign [3]float64
				for a := 0; a < 3; a++ {
					w[a] = 1 - f[a]
					sign[a] = -1
					if b[a] == 1 {
						w[a] = f[a]
						sign[a] = 1
					}
				}
				x := (base[0] + ix) % dims[0]
				y := (base[1] + iy) % dims[1]
				z := (base[2] + iz) % dims[2]
				val := float64(field[z+dims[2]*(y+dims[1]*x)])
				if !finite(val) {
					return 0, grad, fmt.Errorf("nonfinite sampled field")
				}
				result += val * w[0] * w[1] * w[2]
				for a := 0; a < 3; a++ {
					grad[a] += val * sign[a] * w[(a+1)%3] * w[(a+2)%3] / dx
				}
			}
		}
	}
	return result, grad, nil
}

type waveEnergy struct{ Norm, Kinetic, Potential, Nonlinear, Chemical float64 }

func (w waveEnergy) total() float64 { return w.Kinetic + w.Potential + w.Nonlinear + w.Chemical }

// Same discrete Hamiltonian as the FFT kinetic eigenvalue. dw=1 for the
// degenerate single-site lattice is an explicit quadrature convention.
func spectralEnergy(real, imag, potential []float32, dw, hbar, inertia, g, mu float64) (waveEnergy, error) {
	return spectralGeometryEnergy(real, imag, potential, nil, dw, hbar, inertia, g, mu)
}

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
