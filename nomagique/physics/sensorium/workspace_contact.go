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

func (contactMaterial ContactMaterial) validate() error {
	if !contactMaterial.Enabled {
		return nil
	}
	if !isPositiveFinite(contactMaterial.Radius) || !isPositiveFinite(contactMaterial.YoungModulus) || !finite(contactMaterial.PoissonRatio) || contactMaterial.PoissonRatio <= -1 || contactMaterial.PoissonRatio >= .5 || !finite(contactMaterial.RelaxationTime) || contactMaterial.RelaxationTime < 0 || !finite(contactMaterial.Conductivity) || contactMaterial.Conductivity < 0 || !isPositiveFinite(contactMaterial.CV) {
		return fmt.Errorf("invalid Hertz material: %+v", contactMaterial)
	}
	return nil
}

type contactParameters struct {
	N                                                                 uint32
	DT, Radius, Young, Poisson, Relaxation, Conductivity, CV, X, Y, Z float32
}

func (workspace *workspace) contactParams(dt float32) contactParameters {
	c, d := workspace.physics.Contacts, workspace.domain
	return contactParameters{uint32(workspace.particles), dt, float32(c.Radius), float32(c.YoungModulus), float32(c.PoissonRatio), float32(c.RelaxationTime), float32(c.Conductivity), float32(c.CV), float32(d.DomainX), float32(d.DomainY), float32(d.DomainZ)}
}
func (workspace *workspace) contactRates() (float64, error) {
	if !workspace.physics.Contacts.Enabled {
		return 0, nil
	}
	if err := workspace.physics.Contacts.validate(); err != nil {
		return 0, err
	}
	d := workspace.domain
	c := workspace.physics.Contacts
	if d.DomainX <= 4*c.Radius || d.DomainY <= 4*c.Radius || d.DomainZ <= 4*c.Radius {
		return 0, fmt.Errorf("Hertz cutoff requires each periodic extent > 4 radius")
	}
	if err := workspace.engine.ContactHash(workspace.pos, workspace.vel, workspace.mass, workspace.heat, workspace.velOut, workspace.heatOut, workspace.contactReport, workspace.particleStatus, workspace.hydroParams(1), workspace.contactParams(1), true); err != nil {
		return 0, err
	}
	report := workspace.contactReport.Float32Slice()
	bound := workspace.physics.MaxStep
	elastic := 0.
	for i := 0; i < workspace.particles; i++ {
		stiff, thermal := float64(report[7*i+6]), float64(report[7*i+5])
		if stiff > 0 {
			bound = math.Min(bound, .2/stiff)
		}
		if thermal > 0 {
			bound = math.Min(bound, .5/thermal)
		}
		elastic += float64(report[7*i+4])
	}
	workspace.health.Contact = ContactHealth{Enabled: true, ElasticEnergy: elastic, LimitedDT: bound}
	return bound, nil
}
func (workspace *workspace) contactKick(dt float32) error {
	if !workspace.physics.Contacts.Enabled {
		return nil
	}
	if err := workspace.engine.ContactHash(workspace.pos, workspace.vel, workspace.mass, workspace.heat, workspace.velOut, workspace.heatOut, workspace.contactReport, workspace.particleStatus, workspace.hydroParams(dt), workspace.contactParams(dt), false); err != nil {
		return err
	}
	m, q, v, qo, vo := workspace.mass.Float32Slice(), workspace.heat.Float32Slice(), workspace.vel.Float32Slice(), workspace.heatOut.Float32Slice(), workspace.velOut.Float32Slice()
	report := workspace.contactReport.Float32Slice()
	elastic := 0.
	for i := 0; i < workspace.particles; i++ {
		deltaQ := float64(qo[i]) - float64(q[i])
		kinetic := 0.
		for a := 0; a < 3; a++ {
			j := 3*i + a
			kinetic += .5 * float64(m[i]) * (float64(vo[j]) - float64(v[j])) * (float64(vo[j]) + float64(v[j]))
		}
		if err := workspace.materialWork(i, kinetic+deltaQ); err != nil {
			return err
		}
		workspace.health.Sources.ContactToHeat += deltaQ
		workspace.health.Sources.ContactMaterialWork += kinetic + deltaQ
		elastic += float64(report[7*i+4])
	}
	copy(v, vo)
	copy(q, qo)
	workspace.health.Contact.ElasticEnergy = elastic
	workspace.health.Contact.Kicks++
	return nil
}
