package sensorium

import (
	"fmt"
	"math"
)

// ContactMaterial specifies a frictionless, nonadhesive, monodisperse elastic
// material with Hertz normal force, Rayleigh dissipation and contact conduction.
// Enabling this is a constitutive choice; an ideal gas does not imply solid spheres.
// No material constant is guessed. Enabled=false is the deliberate no-contact model.
type ContactMaterial struct {
	Enabled                                                              bool
	Radius, YoungModulus, PoissonRatio, RelaxationTime, Conductivity, CV float64
}

func (c ContactMaterial) validate() error {
	if !c.Enabled {
		return nil
	}
	if !isPositiveFinite(c.Radius) || !isPositiveFinite(c.YoungModulus) || !finite(c.PoissonRatio) || c.PoissonRatio <= -1 || c.PoissonRatio >= .5 || !finite(c.RelaxationTime) || c.RelaxationTime < 0 || !finite(c.Conductivity) || c.Conductivity < 0 || !isPositiveFinite(c.CV) {
		return fmt.Errorf("invalid Hertz material: %+v", c)
	}
	return nil
}

type contactParameters struct {
	N                                                                 uint32
	DT, Radius, Young, Poisson, Relaxation, Conductivity, CV, X, Y, Z float32
}

func (f *workspace) contactParams(dt float32) contactParameters {
	c, d := f.physics.Contacts, f.domain
	return contactParameters{uint32(f.particles), dt, float32(c.Radius), float32(c.YoungModulus), float32(c.PoissonRatio), float32(c.RelaxationTime), float32(c.Conductivity), float32(c.CV), float32(d.DomainX), float32(d.DomainY), float32(d.DomainZ)}
}
func (f *workspace) contactRates() (float64, error) {
	if !f.physics.Contacts.Enabled {
		return 0, nil
	}
	if err := f.physics.Contacts.validate(); err != nil {
		return 0, err
	}
	d := f.domain
	c := f.physics.Contacts
	if d.DomainX <= 4*c.Radius || d.DomainY <= 4*c.Radius || d.DomainZ <= 4*c.Radius {
		return 0, fmt.Errorf("Hertz cutoff requires each periodic extent > 4 radius")
	}
	if err := f.engine.ContactHash(f.pos, f.vel, f.mass, f.heat, f.velOut, f.heatOut, f.contactReport, f.particleStatus, f.hydroParams(1), f.contactParams(1), true); err != nil {
		return 0, err
	}
	report := f.contactReport.Float32Slice()
	bound := f.physics.MaxStep
	elastic := 0.
	for i := 0; i < f.particles; i++ {
		stiff, thermal := float64(report[7*i+6]), float64(report[7*i+5])
		if stiff > 0 {
			bound = math.Min(bound, .2/stiff)
		}
		if thermal > 0 {
			bound = math.Min(bound, .5/thermal)
		}
		elastic += float64(report[7*i+4])
	}
	f.health.Contact = ContactHealth{Enabled: true, ElasticEnergy: elastic, LimitedDT: bound}
	return bound, nil
}
func (f *workspace) contactKick(dt float32) error {
	if !f.physics.Contacts.Enabled {
		return nil
	}
	if err := f.engine.ContactHash(f.pos, f.vel, f.mass, f.heat, f.velOut, f.heatOut, f.contactReport, f.particleStatus, f.hydroParams(dt), f.contactParams(dt), false); err != nil {
		return err
	}
	m, q, v, qo, vo := f.mass.Float32Slice(), f.heat.Float32Slice(), f.vel.Float32Slice(), f.heatOut.Float32Slice(), f.velOut.Float32Slice()
	report := f.contactReport.Float32Slice()
	elastic := 0.
	for i := 0; i < f.particles; i++ {
		deltaQ := float64(qo[i]) - float64(q[i])
		kinetic := 0.
		for a := 0; a < 3; a++ {
			j := 3*i + a
			kinetic += .5 * float64(m[i]) * (float64(vo[j]) - float64(v[j])) * (float64(vo[j]) + float64(v[j]))
		}
		if err := f.materialWork(i, kinetic+deltaQ); err != nil {
			return err
		}
		f.health.Sources.ContactToHeat += deltaQ
		f.health.Sources.ContactMaterialWork += kinetic + deltaQ
		elastic += float64(report[7*i+4])
	}
	copy(v, vo)
	copy(q, qo)
	f.health.Contact.ElasticEnergy = elastic
	f.health.Contact.Kicks++
	return nil
}
