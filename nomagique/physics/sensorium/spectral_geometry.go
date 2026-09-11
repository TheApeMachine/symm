package sensorium

import "fmt"

// SetSpectralGeometry specifies a real scalar potential and the positive proper-
// volume density w=sqrt(g) on the periodic omega grid. Nil means V=0 or w=1.
// State stores canonical z=sqrt(w)*psi; sum |z|^2*domega is physical norm.
// Setting geometry after evolution would require a parameter-work/coordinate
// transformation operator, so this initializer rejects that ambiguous operation.
// No market geometry is inferred, and no nontrivial metric is enabled by default.
func (m *Manifold) SetSpectralGeometry(potential, metricVolume []float32) error {
	if m == nil {
		return fmt.Errorf("nil manifold")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	f := m.work
	if f == nil {
		return fmt.Errorf("closed manifold")
	}
	if f.physicalTime != 0 {
		return fmt.Errorf("spectral geometry is immutable during an evolved trajectory")
	}
	n := int(f.domain.MaxModes)
	for k, v := range [][]float32{potential, metricVolume} {
		if len(v) != 0 && len(v) != n {
			return fmt.Errorf("spectral geometry length must be zero or %d", n)
		}
		for i, x := range v {
			if !finite(float64(x)) || (k == 1 && x <= 0) {
				return fmt.Errorf("invalid spectral geometry field %d node %d", k, i)
			}
		}
	}
	f.engine.Synchronize()
	// Allocate and initialize both candidates before replacing owned storage.
	var p, w *Buffer
	if len(potential) > 0 {
		p = f.gpu(uint64(n) * 4)
		if p == nil {
			return fmt.Errorf("allocate scalar potential")
		}
		copy(p.Float32Slice(), potential)
	}
	if len(metricVolume) > 0 {
		w = f.gpu(uint64(n) * 4)
		if w == nil {
			if p != nil {
				p.Close()
			}
			return fmt.Errorf("allocate metric volume")
		}
		copy(w.Float32Slice(), metricVolume)
	}
	if f.spectralPotential != nil {
		f.spectralPotential.Close()
	}
	if f.spectralMetric != nil {
		f.spectralMetric.Close()
	}
	f.spectralPotential, f.spectralMetric = p, w
	for h := 0; h < spectralHeads; h++ {
		for i := 0; i < n; i++ {
			f.previousPotential[h*n+i] = 0
			if p != nil {
				f.previousPotential[h*n+i] = potential[i]
			}
		}
	}
	return nil
}
