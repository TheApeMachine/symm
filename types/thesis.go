package types

import (
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Thesis is essentially the "state" of a tick. It travels across the
entire lifecycle of a tick, picking up all data along the way.
*/
type Thesis struct {
	uiHub        chan<- []byte
	Signals      *sync.Map
	CrossSection *CrossSection
	Measurements []*Measurement
	Graphs       map[string]*Graph
	Forecasts    []Forecasts
	Hypotheses   []Hypothesis
	Categories   []Category
}

/*
NewThesis creates an empty in-process lifecycle carrier for one tick.
*/
func NewThesis(uiHub chan<- []byte) *Thesis {
	crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	return &Thesis{
		uiHub:        uiHub,
		Signals:      &sync.Map{},
		CrossSection: crossSection,
		Measurements: make([]*Measurement, 0),
		Graphs:       make(map[string]*Graph),
		Forecasts:    make([]Forecasts, 0),
		Hypotheses:   make([]Hypothesis, 0),
		Categories:   make([]Category, 0),
	}
}

func (thesis *Thesis) Publish() {
	select {
	case thesis.uiHub <- datura.Map[any]{
		"diagnostics": []CrossSectionSummary{
			*thesis.CrossSection.ReadView(),
		},
		"measurements": thesis.Measurements,
	}.Marshal():
	default:
	}
}
