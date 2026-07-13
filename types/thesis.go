package types

import (
	"sync"

	"github.com/theapemachine/datura"
)

/*
Thesis is essentially the "state" of a tick. It travels across the
entire lifecycle of a tick, picking up all data along the way.
*/
type Thesis struct {
	uiHub        chan<- []byte
	Signals      *sync.Map
	CrossSection *CrossSection
	Measurements *sync.Map
}

/*
NewThesis creates an empty in-process lifecycle carrier for one tick.
*/
func NewThesis(uiHub chan<- []byte) *Thesis {
	crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

	if err != nil {
		panic(err)
	}

	return &Thesis{
		uiHub:        uiHub,
		Signals:      &sync.Map{},
		CrossSection: crossSection,
		Measurements: &sync.Map{},
	}
}

func (thesis *Thesis) Publish() {
	out := datura.Map[any]{
		"cross_section": thesis.CrossSection,
	}

	thesis.Measurements.Range(func(key, value any) bool {
		out[key.(string)] = value
		return true
	})

	select {
	case thesis.uiHub <- out.Marshal():
	default:
	}
}
