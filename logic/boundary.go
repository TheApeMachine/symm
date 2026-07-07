package logic

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

var errBoundaryNoClamps = errors.New("decision boundary: no metric clamps available")

var boundarySourceOrder = []types.SourceType{
	types.SourceHawkes,
	types.SourceCVD,
	types.SourceFluid,
	types.SourceDepthFlow,
	types.SourceLiquidity,
	types.SourcePumpDump,
	types.SourceExhaustion,
	types.SourceToxicity,
	types.SourceLeadLag,
	types.SourceCorrelation,
	types.SourceSentiment,
}

type boundaryClamps struct {
	lanes     map[types.SourceType]int
	projector *clampProjector
}

type boundaryFrame struct {
	symbol      string
	clamps      []fieldClamp
	oscillators []pmanifold.Oscillator
	price       decimal.Decimal
	eventAt     time.Time
}

type fieldClamp struct {
	source    types.SourceType
	category  types.CategoryType
	lane      int
	positionX float64
	positionZ float64
	rho       float64
	momX      float64
	momY      float64
	momZ      float64
	energy    float64
	pressure  float64
}

func newBoundaryClamps() *boundaryClamps {
	lanes := make(map[types.SourceType]int, len(boundarySourceOrder))

	for index, source := range boundarySourceOrder {
		lanes[source] = index
	}

	return &boundaryClamps{
		lanes:     lanes,
		projector: newClampProjector(),
	}
}

func (boundaries *boundaryClamps) Frame(
	symbol string,
	measurements map[types.SourceType]*types.Measurement,
) (boundaryFrame, error) {
	if strings.TrimSpace(symbol) == "" {
		return boundaryFrame{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision boundary: symbol required",
			nil,
		))
	}

	if len(measurements) == 0 {
		return boundaryFrame{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision boundary: measurements required",
			nil,
		))
	}

	frame := boundaryFrame{symbol: symbol}
	var rejected error

	for source, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if strings.TrimSpace(measurement.Symbol) != symbol {
			continue
		}

		clamp, err := boundaries.clamp(source, measurement)

		if err != nil {
			rejected = errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))

			boundaries.reject(source, err)
			continue
		}

		frame.clamps = append(frame.clamps, clamp)
		frame.oscillators = append(frame.oscillators, clamp.oscillator())
		frame.observe(measurement)
	}

	if len(frame.clamps) == 0 {
		return boundaryFrame{}, boundaryNoClampsError{cause: rejected}
	}

	if frame.eventAt.IsZero() {
		return boundaryFrame{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"decision boundary: measurement time required",
			nil,
		))
	}

	return frame, nil
}

func (boundaries *boundaryClamps) clamp(
	source types.SourceType,
	measurement *types.Measurement,
) (fieldClamp, error) {
	lane, ok := boundaries.lanes[source]
	if !ok {
		return fieldClamp{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("decision boundary: unknown source %q", source),
			nil,
		))
	}

	category := bestCategory(measurement.Categories)
	if category.Type == types.CategoryTypeNone {
		return fieldClamp{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("decision boundary: %s category required", source),
			nil,
		))
	}

	sample, err := boundaries.projector.Project(source, measurement)
	if err != nil {
		return fieldClamp{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("decision boundary: %s clamp failed to project", source),
			err,
		))
	}

	if sample.rho <= 0 || sample.energy <= 0 {
		return fieldClamp{}, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("decision boundary: %s clamp has no mass-energy", source),
			nil,
		))
	}

	return fieldClamp{
		source:    source,
		category:  category.Type,
		lane:      lane,
		positionX: signedPosition(sample.momX),
		positionZ: pressurePosition(sample.momY, sample.pressure),
		rho:       sample.rho,
		momX:      sample.momX,
		momY:      sample.momY,
		momZ:      sample.momZ,
		energy:    sample.energy,
		pressure:  sample.pressure,
	}, nil
}
