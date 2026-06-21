package sentiment

import (
	"context"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	marketsection "github.com/theapemachine/symm/market"
)

/*
Signal measures global market conviction from breadth and leadership performance.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriteCloser
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
}

/*
NewSignal composes the conviction pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: time.Minute,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	})

	if err != nil {
		cancel()

		return nil
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		tree:         tree,
		CrossSection: crossSection,
		algo: nomagique.Number(
			equation.NewConviction(), probability.NewClassifier(
				datura.Acquire("sentiment-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"surgeScore", "divergentScore", "slumpScore"},
				}),
			),
		),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil || signal.CrossSection == nil {
		return nil
	}

	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(signal.CrossSection.Observe(row)) != nil {
		return nil
	}

	breadth := signal.CrossSection.Breadth(row.Updated)
	signal.CrossSection.RecordBreadth(breadth)

	leaderFlag := 0.0

	if signal.CrossSection.IsLeader(row.Name, row.Value, row.Updated) {
		leaderFlag = 1
	}

	features := []float64{
		breadth,
		row.Value,
		signal.CrossSection.MajorityThreshold(row.Updated),
		leaderFlag,
		row.Value,
	}

	stored := datura.Acquire("sentiment-conviction", datura.APPJSON)
	stored.WithPayload(equation.MarshalFeaturesPayload(features))

	if errnie.Error(transport.NewFlipFlop(
		stored, signal.algo,
	)) != nil {
		stored.Release()

		return nil
	}

	confidence := datura.Peek[float64](stored, "output", "confidence")
	uniformConfidence := 1.0 / 3.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		stored.Release()

		return nil
	}

	return stored
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
